// Copyright 2026 The Kstack Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sandbox

import (
	"crypto/rand"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitArgsAreTheSocketThePortAndTheCommand(t *testing.T) {
	got, err := parseInitArgs([]string{"--socket", "/run/p.sock", "--port", "6443", "--", "/bin/sh", "-c", "--port 1"})
	require.NoError(t, err)
	assert.Equal(t, initArgs{socket: "/run/p.sock", port: 6443, argv: []string{"/bin/sh", "-c", "--port 1"}}, got)

	got, err = parseInitArgs([]string{"--port=1", "--socket=s", "--", "x"})
	require.NoError(t, err)
	assert.Equal(t, initArgs{socket: "s", port: 1, argv: []string{"x"}}, got)
}

// echoSocket is a Unix socket, in a short directory of its own so its path
// fits, whose server echoes each connection and writes |eof once the client's
// input ends, then ends its own.
func echoSocket(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "relay")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "s")
	ln, err := net.Listen("unix", socket)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = io.Copy(c, c)
				_, _ = io.WriteString(c, "|eof")
				_ = c.(*net.UnixConn).CloseWrite()
			}()
		}
	}()
	return socket
}

// The relay carries bytes both ways unchanged, and passes each side's
// half-close to the other: the echo server sees the client's end of input,
// and the client sees the server's.
func TestTheRelayIsByteForByte(t *testing.T) {
	socket := echoSocket(t)
	tcpLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = tcpLn.Close() })
	go func() {
		c, err := tcpLn.Accept()
		if err == nil {
			relay(c, socket)
		}
	}()

	client, err := net.Dial("tcp", tcpLn.Addr().String())
	require.NoError(t, err)
	defer client.Close()
	sent := make([]byte, 1<<20)
	_, _ = rand.Read(sent)
	go func() {
		_, _ = client.Write(sent)
		_ = client.(*net.TCPConn).CloseWrite()
	}()
	got, err := io.ReadAll(client)

	require.NoError(t, err)
	assert.Equal(t, append(sent, "|eof"...), got)
}

// A connection whose socket cannot be reached is closed, so its client sees
// the end at once.
func TestTheRelayClosesWhatItCannotCarry(t *testing.T) {
	client, relayed := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		relay(relayed, filepath.Join(t.TempDir(), "missing"))
	}()

	_, err := client.Read(make([]byte, 1))

	assert.ErrorIs(t, err, io.EOF)
	<-done
}

// A run with no cluster names neither flag, and its forwarder listens on
// nothing.
func TestInitArgsTakeNeitherFlag(t *testing.T) {
	got, err := parseInitArgs([]string{"--", "/bin/sh", "-c", "true"})
	require.NoError(t, err)
	assert.Equal(t, initArgs{argv: []string{"/bin/sh", "-c", "true"}}, got)
}

func TestInitArgsRefuseWhatIsMissing(t *testing.T) {
	for name, args := range map[string][]string{
		"no socket":           {"--port", "1", "--", "x"},
		"no port":             {"--socket", "s", "--", "x"},
		"neither, no command": {"--"},
		"port zero":           {"--socket", "s", "--port", "0", "--", "x"},
		"port too big":        {"--socket", "s", "--port", "65536", "--", "x"},
		"no command":          {"--socket", "s", "--port", "1", "--"},
		"an unknown":          {"--socket", "s", "--port", "1", "--other", "--", "x"},
	} {
		_, err := parseInitArgs(args)
		assert.Error(t, err, name)
	}
}

// The forwarder's command line parses back to the run it was written for,
// with or without a socket.
func TestForwarderArgsRoundTrip(t *testing.T) {
	r := Run{Shell: "/bin/sh", Args: []string{"-c", "--port 1"}, Socket: "/run/p.sock", Port: 6443}
	args := ForwarderArgs(r)
	require.Equal(t, InitCommand, args[0])
	got, err := parseInitArgs(append(append(args[1:], r.Shell), r.Args...))
	require.NoError(t, err)
	assert.Equal(t, initArgs{socket: r.Socket, port: 6443, argv: []string{"/bin/sh", "-c", "--port 1"}}, got)

	r.Socket = ""
	got, err = parseInitArgs(append(ForwarderArgs(r)[1:], r.Shell))
	require.NoError(t, err)
	assert.Equal(t, initArgs{argv: []string{"/bin/sh"}}, got)
}
