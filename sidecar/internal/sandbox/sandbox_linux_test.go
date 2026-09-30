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
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The first system bwrap that exists, in the list's order, comes first; then
// Kstack's own, beside its executable; none where none exists.
func TestTheSystemBwrapIsFoundFirst(t *testing.T) {
	base := t.TempDir()
	bin := filepath.Join(base, "usr", "bin")
	system := []string{filepath.Join(base, "a", "bwrap"), filepath.Join(base, "b", "bwrap"), filepath.Join(base, "c", "bwrap")}
	write(t, system[1], "")
	write(t, system[2], "")

	assert.Empty(t, bwrapPaths(bin, system[:1]))
	assert.Equal(t, []string{system[1]}, bwrapPaths(bin, system))

	own := write(t, filepath.Join(base, "usr", "lib", "kstack", "bwrap"), "")
	assert.Equal(t, []string{system[1], own}, bwrapPaths(bin, system))
	assert.Equal(t, []string{own}, bwrapPaths(bin, system[:1]))
}

// seq reports whether args holds want as a run of consecutive arguments.
func seq(args []string, want ...string) bool {
	for i := range args {
		if i+len(want) <= len(args) && slices.Equal(args[i:i+len(want)], want) {
			return true
		}
	}
	return false
}

// A run starts bwrap with the run's environment, its namespaces first and the
// chain last: the forwarder, then the shell launcher, then the shell.
func TestTheCommandStartsBwrapOverTheChain(t *testing.T) {
	s := &Sandbox{self: "/opt/k/kstack-sidecar", bwrap: "/usr/bin/bwrap"}
	r := Run{
		Shell: "/bin/bash", Args: []string{"-c", "ls"}, Dir: "/w/sub", Env: []string{"A=1"},
		Workspace: "/w", Readable: []string{"/snap.sh", "/run/r1"}, Writable: []string{"/c/tmp/1", "/c/kubectl"},
		Socket: "/run/r1/proxy.sock", Port: 6443,
	}

	cmd := s.Command(context.Background(), r)

	assert.Equal(t, "/usr/bin/bwrap", cmd.Path)
	assert.Equal(t, []string{"A=1"}, cmd.Env)
	args := cmd.Args[1:]
	assert.Equal(t, []string{
		"--unshare-net", "--unshare-pid", "--unshare-ipc", "--unshare-uts", "--unshare-cgroup-try",
		"--die-with-parent", "--new-session", "--as-pid-1",
	}, args[:8])
	assert.True(t, seq(args, "--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp"), args)
	for _, p := range []string{"/snap.sh", "/run/r1", "/opt/k/kstack-sidecar"} {
		assert.True(t, seq(args, "--ro-bind", p, p), p)
	}
	for _, p := range []string{"/w", "/c/tmp/1", "/c/kubectl"} {
		assert.True(t, seq(args, "--bind", p, p), p)
	}
	assert.True(t, seq(args, "--remount-ro", "/"), args)
	assert.Equal(t, []string{
		"--chdir", "/w/sub", "--",
		"/opt/k/kstack-sidecar", InitCommand, "--socket", "/run/r1/proxy.sock", "--port", "6443", "--",
		"/opt/k/kstack-sidecar", ShellCommand, "--", "/bin/bash", "-c", "ls",
	}, args[len(args)-16:])
}

// A run with no cluster names no socket, and its forwarder listens on nothing.
func TestARunWithNoSocketStartsTheForwarderWithNoFlags(t *testing.T) {
	s := &Sandbox{self: "/k", bwrap: "/usr/bin/bwrap"}

	args := s.Command(context.Background(), Run{Shell: "/bin/sh", Args: []string{"-c", "true"}, Dir: "/w", Workspace: "/w"}).Args

	assert.Equal(t, []string{"--", "/k", InitCommand, "--", "/k", ShellCommand, "--", "/bin/sh", "-c", "true"}, args[len(args)-10:])
	assert.False(t, slices.Contains(args, "--socket"))
}

// Each root that exists is bound read-only at its own path, and one that
// does not, or links to nothing, is left out. A root that is a link is
// recreated as one, and its target is bound unless another root holds it.
func TestTheRootsAreBound(t *testing.T) {
	base := resolved(t.TempDir())
	d := mkdirs(t, base, "usr/bin", "var/opt")
	usr, varOpt := filepath.Join(base, "usr"), d[1]
	bin := filepath.Join(base, "bin")
	require.NoError(t, os.Symlink("usr/bin", bin))
	opt := filepath.Join(base, "opt")
	require.NoError(t, os.Symlink(varOpt, opt))
	dangling := filepath.Join(base, "lib64")
	require.NoError(t, os.Symlink("usr/lib64", dangling))
	missing := filepath.Join(base, "lib32")

	args, bound := rootArgs([]string{usr, bin, opt, dangling, missing})

	assert.Equal(t, []string{
		"--ro-bind", usr, usr,
		"--symlink", d[0], bin,
		"--ro-bind", varOpt, varOpt, "--symlink", varOpt, opt,
	}, args)
	assert.Equal(t, []string{usr, varOpt}, bound)
}

// fakeBwrap is a script standing in for bwrap at path, running body.
func fakeBwrap(t *testing.T, path, body string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700))
	return path
}

func TestWithoutBwrapThereIsNoSandbox(t *testing.T) {
	s, v := probe(t.Context(), os.Args[0], nil, time.Minute)

	assert.Nil(t, s)
	assert.Equal(t, Verdict{Reason: "bwrap not found"}, v)
}

// A bwrap that fails the probe is no sandbox, and the reason is the first
// line it wrote, which names the cause.
func TestAProbeThatFailsSaysWhy(t *testing.T) {
	bwrap := fakeBwrap(t, filepath.Join(t.TempDir(), "bwrap"),
		`echo "bwrap: setting up uid map: Permission denied" >&2; echo more >&2; exit 1`)

	s, v := probe(t.Context(), os.Args[0], []string{bwrap}, time.Minute)

	assert.Nil(t, s)
	assert.Equal(t, Verdict{Reason: bwrap + ": bwrap: setting up uid map: Permission denied"}, v)
}

// A probe past its bound fails, saying so.
func TestAProbePastItsBoundFails(t *testing.T) {
	// Latency injected into the code under test: the fake outlasts the bound.
	bwrap := fakeBwrap(t, filepath.Join(t.TempDir(), "bwrap"), "exec sleep 60")

	s, v := probe(t.Context(), os.Args[0], []string{bwrap}, 10*time.Millisecond)

	assert.Nil(t, s)
	assert.Equal(t, Verdict{Reason: bwrap + ": no answer in 10ms"}, v)
}

// When the system's bwrap fails, Kstack's own is probed in its place, and
// both reasons are named when both fail; one that passes is never replaced.
func TestKstacksOwnBwrapStandsInForTheSystems(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "own-ran")
	failing := fakeBwrap(t, filepath.Join(dir, "system-failing"), `echo "system failed" >&2; exit 1`)
	passing := fakeBwrap(t, filepath.Join(dir, "system-passing"), "exit 0")
	own := fakeBwrap(t, filepath.Join(dir, "own"), "touch "+marker)
	ownFailing := fakeBwrap(t, filepath.Join(dir, "own-failing"), `echo "own failed" >&2; exit 1`)

	s, v := probe(t.Context(), os.Args[0], []string{failing, own}, time.Minute)
	require.NotNil(t, s)
	assert.Equal(t, own, s.bwrap)
	assert.Equal(t, Verdict{Available: true, Reason: "bwrap at " + own}, v)

	require.NoError(t, os.Remove(marker))
	s, _ = probe(t.Context(), os.Args[0], []string{passing, own}, time.Minute)
	require.NotNil(t, s)
	assert.Equal(t, passing, s.bwrap)
	assert.NoFileExists(t, marker)

	s, v = probe(t.Context(), os.Args[0], []string{failing, ownFailing}, time.Minute)
	assert.Nil(t, s)
	assert.Equal(t, Verdict{Reason: failing + ": system failed; " + ownFailing + ": own failed"}, v)
}

// A probe with nowhere to make its workspace fails, saying why.
func TestAProbeWithNoTempDirFails(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	bwrap := fakeBwrap(t, filepath.Join(t.TempDir(), "bwrap"), "exit 0")

	s, v := probe(t.Context(), os.Args[0], []string{bwrap}, time.Minute)

	assert.Nil(t, s)
	assert.Contains(t, v.Reason, "no such file or directory")
}

// A bwrap that fails without a word is named by how it exited.
func TestAProbeThatFailsSilentlySaysHowItExited(t *testing.T) {
	bwrap := fakeBwrap(t, filepath.Join(t.TempDir(), "bwrap"), "exit 3")

	_, v := probe(t.Context(), os.Args[0], []string{bwrap}, time.Minute)

	assert.Equal(t, Verdict{Reason: bwrap + ": exit status 3"}, v)
}

// run starts r through s and answers its exit code and its stdout. Stderr
// is left out of the answer: under coverage the forwarder, this test binary,
// warns there as it exits, since its GOCOVERDIR is outside the sandbox.
func run(t *testing.T, s *Sandbox, r Run) (int, string) {
	t.Helper()
	cmd := s.Command(t.Context(), r)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if cmd.ProcessState == nil {
		require.NoError(t, err)
	}
	t.Log("stderr: ", stderr.String())
	return ExitCode(cmd.ProcessState), string(out)
}

// shRun is a run of script under /bin/sh in a fresh workspace, with a PATH
// of the system's and the environment env adds.
func shRun(t *testing.T, script string, env ...string) Run {
	t.Helper()
	ws := resolved(t.TempDir())
	return Run{
		Shell: "/bin/sh", Args: []string{"-c", script}, Dir: ws, Workspace: ws,
		Env: append([]string{"PATH=/usr/bin:/bin"}, env...), Home: resolved(t.TempDir()),
	}
}

// The command sees exactly the environment it was given: nothing the
// forwarder, the shell launcher or bwrap has of its own.
func TestTheEnvironmentIsExactlyGiven(t *testing.T) {
	s := confining(t)

	code, out := run(t, s, shRun(t, "env | grep -v -e ^PWD= -e ^SHLVL= -e ^_= | sort", "ONLY=this"))

	assert.Equal(t, 0, code, out)
	assert.Equal(t, "ONLY=this\nPATH=/usr/bin:/bin\n", out)
}

// bwrap confines, and a run's forwarder listens on one fixed port, since the
// namespace's loopback is the run's own.
func TestBwrapConfinesOnAFixedPort(t *testing.T) {
	s := &Sandbox{self: "/k", bwrap: "/usr/bin/bwrap"}

	port, err := s.Port()

	require.NoError(t, err)
	assert.Equal(t, forwarderPort, port)
	assert.True(t, s.Confines())
}

// index is where want starts as a run in args, or -1.
func index(args []string, want ...string) int {
	for i := range args {
		if i+len(want) <= len(args) && slices.Equal(args[i:i+len(want)], want) {
			return i
		}
	}
	return -1
}

// The mounts come in the order that lets each lie over the last: the PATH
// trees and links, then the denials over them, then the run's own, then the
// read-only remounts, which come after the binds made inside a denial.
func TestTheCommandMountsInOrder(t *testing.T) {
	home, _, outside := machine(t)
	cargo := mkdirs(t, home, ".cargo/bin")[0]
	creds := filepath.Join(home, ".cargo", "credentials.toml")
	require.NoError(t, os.WriteFile(creds, nil, 0o600))
	data := mkdirs(t, home, ".cargo/kstack-data")[0]
	ws := mkdirs(t, data, "chats/1/workspace")[0]
	tool := mkdirs(t, outside, "tool/bin")[0]
	links := mkdirs(t, outside, "links")[0]
	require.NoError(t, os.Symlink(tool, filepath.Join(links, "tool")))
	shellDir := mkdirs(t, outside, "shells")[0]
	s := &Sandbox{self: "/k", bwrap: "/usr/bin/bwrap"}

	args := s.Command(context.Background(), Run{
		Shell: filepath.Join(shellDir, "bash"), Args: []string{"-c", "true"}, Dir: ws, Workspace: ws,
		Env:  []string{"PATH=" + cargo + ":" + filepath.Join(links, "tool")},
		Home: home, Denied: []string{data, filepath.Join(outside, "runtime")},
	}).Args

	tree := index(args, "--ro-bind", filepath.Join(home, ".cargo"), filepath.Join(home, ".cargo"))
	shell := index(args, "--ro-bind", shellDir, shellDir)
	symlink := index(args, "--symlink", tool, filepath.Join(links, "tool"))
	cred := index(args, "--ro-bind", "/dev/null", creds)
	denial := index(args, "--tmpfs", data)
	own := index(args, "--bind", ws, ws)
	root := index(args, "--remount-ro", "/")
	sealed := index(args, "--remount-ro", data)
	for name, i := range map[string]int{"tree": tree, "shell": shell, "symlink": symlink, "cred": cred, "denial": denial, "own": own, "root": root, "sealed": sealed} {
		require.NotEqual(t, -1, i, "%s missing from %v", name, args)
	}
	assert.Less(t, tree, symlink)
	assert.Less(t, symlink, cred)
	assert.Less(t, cred, denial)
	assert.Less(t, denial, own)
	assert.Less(t, own, root)
	assert.Less(t, root, sealed)
	assert.Equal(t, -1, index(args, "--tmpfs", filepath.Join(outside, "runtime")), "a denial no tree reaches needs no mount")
}

// A root with a link above it is bound at its resolved path and recreated as
// a link at its own, as Homebrew's is under Fedora Atomic's linked /home.
func TestARootBelowALinkIsBoundResolved(t *testing.T) {
	base := resolved(t.TempDir())
	brew := mkdirs(t, base, "var/home/linuxbrew/.linuxbrew")[0]
	require.NoError(t, os.Symlink(filepath.Join(base, "var", "home"), filepath.Join(base, "home")))
	root := filepath.Join(base, "home", "linuxbrew", ".linuxbrew")

	args, _ := rootArgs([]string{root})

	assert.Equal(t, []string{"--ro-bind", brew, brew, "--symlink", brew, root}, args)
}

// write makes a file at path holding text, and its directory.
func write(t *testing.T, path, text string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(text), 0o700))
	return path
}

// program makes an executable script at path that prints name.
func program(t *testing.T, path, name string) string {
	t.Helper()
	return write(t, path, "#!/bin/sh\necho "+name+"\n")
}

// withPath is r with PATH set to the system's then entries.
func withPath(r Run, entries ...string) Run {
	r.Env = append([]string{"PATH=" + strings.Join(append([]string{"/usr/bin", "/bin"}, entries...), ":")}, r.Env[1:]...)
	return r
}

func TestTheHomeIsUnreadable(t *testing.T) {
	s := confining(t)
	r := shRun(t, "")
	write(t, filepath.Join(r.Home, "secret"), "secret")
	r.Args[1] = "cat " + filepath.Join(r.Home, "secret") + "; ls -A " + r.Home

	_, out := run(t, s, r)

	assert.NotContains(t, out, "secret")
}

func TestACredentialPathInsideAReadableTreeIsUnreadable(t *testing.T) {
	s := confining(t)
	r := shRun(t, "")
	program(t, filepath.Join(r.Home, ".cargo", "bin", "tool"), "tool-ran")
	creds := write(t, filepath.Join(r.Home, ".cargo", "credentials.toml"), "secret")
	r = withPath(r, filepath.Join(r.Home, ".cargo", "bin"))
	r.Args[1] = "tool; cat " + creds

	_, out := run(t, s, r)

	assert.Contains(t, out, "tool-ran")
	assert.NotContains(t, out, "secret")
}

func TestLocalShareIsUnreadable(t *testing.T) {
	s := confining(t)
	r := shRun(t, "")
	bin := filepath.Join(r.Home, ".local", "bin")
	program(t, filepath.Join(bin, "tool"), "tool-ran")
	x := program(t, filepath.Join(r.Home, ".local", "share", "pipx", "venvs", "x", "bin", "x"), "pipx-ran")
	require.NoError(t, os.Symlink(x, filepath.Join(bin, "x")))
	other := write(t, filepath.Join(r.Home, ".local", "share", "other", "x"), "secret")
	state := write(t, filepath.Join(r.Home, ".local", "state", "x"), "secret")
	r = withPath(r, bin)
	r.Args[1] = "tool; x; cat " + other + " " + state

	_, out := run(t, s, r)

	assert.Contains(t, out, "tool-ran")
	assert.Contains(t, out, "pipx-ran")
	assert.NotContains(t, out, "secret")
}

// A ~/.local/share that links elsewhere in the home opens the tool's tree
// there, and nothing beside it.
func TestALinkedLocalShareOpensOnlyTheToolsTree(t *testing.T) {
	s := confining(t)
	r := shRun(t, "")
	docs := filepath.Join(r.Home, "Documents")
	program(t, filepath.Join(docs, "pipx", "bin", "x"), "pipx-ran")
	private := write(t, filepath.Join(docs, "private", "notes"), "secret")
	require.NoError(t, os.MkdirAll(filepath.Join(r.Home, ".local"), 0o700))
	require.NoError(t, os.Symlink(docs, filepath.Join(r.Home, ".local", "share")))
	r = withPath(r, filepath.Join(r.Home, ".local", "share", "pipx", "bin"))
	r.Args[1] = "x; cat " + private

	_, out := run(t, s, r)

	assert.Contains(t, out, "pipx-ran")
	assert.NotContains(t, out, "secret")
}

// A program reached through a chain of links runs.
func TestAProgramLinkChainRuns(t *testing.T) {
	s := confining(t)
	r := shRun(t, "tool")
	tool := program(t, filepath.Join(r.Home, "install", "bin", "tool"), "tool-ran")
	bin := filepath.Join(r.Home, ".local", "bin")
	require.NoError(t, os.MkdirAll(bin, 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(r.Home, "links"), 0o700))
	require.NoError(t, os.Symlink(tool, filepath.Join(r.Home, "links", "tool")))
	require.NoError(t, os.Symlink(filepath.Join(r.Home, "links", "tool"), filepath.Join(bin, "tool")))

	code, out := run(t, s, withPath(r, bin))

	assert.Equal(t, 0, code)
	assert.Equal(t, "tool-ran\n", out)
}

// linkedHome is a home reached through a link: the link, and the directory
// it names.
func linkedHome(t *testing.T) (link, home string) {
	t.Helper()
	base := resolved(t.TempDir())
	home = mkdirs(t, base, "data/ana")[0]
	link = filepath.Join(base, "ana")
	require.NoError(t, os.Symlink(home, link))
	return link, home
}

func TestAProgramLinkThroughALinkedHomeRuns(t *testing.T) {
	s := confining(t)
	link, _ := linkedHome(t)
	r := shRun(t, "x")
	r.Home = link
	x := program(t, filepath.Join(link, ".local", "share", "pipx", "venvs", "x", "bin", "x"), "pipx-ran")
	bin := filepath.Join(link, ".local", "bin")
	require.NoError(t, os.MkdirAll(bin, 0o700))
	require.NoError(t, os.Symlink(x, filepath.Join(bin, "x")))
	r = withPath(r, bin)

	code, out := run(t, s, r)

	assert.Equal(t, 0, code)
	assert.Equal(t, "pipx-ran\n", out)
}

func TestKstacksDirectoriesAreUnreadableThroughALinkedHome(t *testing.T) {
	s := confining(t)
	link, home := linkedHome(t)
	r := shRun(t, "")
	r.Home = link
	program(t, filepath.Join(home, ".config", "tool", "bin", "tool"), "tool-ran")
	write(t, filepath.Join(home, ".config", "kstack", "app.db"), "secret")
	r.Denied = []string{filepath.Join(link, ".config", "kstack")}
	r = withPath(r, filepath.Join(link, ".config", "tool", "bin"))
	r.Args[1] = "tool; cat " + filepath.Join(home, ".config", "kstack", "app.db") + " " + filepath.Join(link, ".config", "kstack", "app.db")

	_, out := run(t, s, r)

	assert.Contains(t, out, "tool-ran")
	assert.NotContains(t, out, "secret")
}

func TestAPathEntryThroughALinkRuns(t *testing.T) {
	s := confining(t)
	r := shRun(t, "nixtool; tool")
	outside := resolved(t.TempDir())
	program(t, filepath.Join(outside, "profile", "bin", "nixtool"), "nix-ran")
	require.NoError(t, os.Symlink(filepath.Join(outside, "profile"), filepath.Join(r.Home, ".nix-profile")))
	program(t, filepath.Join(outside, "tool", "bin", "tool"), "tool-ran")
	require.NoError(t, os.MkdirAll(filepath.Join(outside, "links"), 0o700))
	require.NoError(t, os.Symlink(filepath.Join(outside, "tool", "bin"), filepath.Join(outside, "links", "tool")))
	r = withPath(r, filepath.Join(r.Home, ".nix-profile", "bin"), filepath.Join(outside, "links", "tool"))

	code, out := run(t, s, r)

	assert.Equal(t, 0, code)
	assert.Equal(t, "nix-ran\ntool-ran\n", out)
}

// addRoot makes dir one of the system roots for the rest of the test.
func addRoot(t *testing.T, dir string) {
	t.Helper()
	old := roots
	roots = append(slices.Clone(roots), dir)
	t.Cleanup(func() { roots = old })
}

// A credential that is a link is unreadable where its target lies, though a
// PATH tree takes that in.
func TestACredentialThroughALinkIsUnreadable(t *testing.T) {
	s := confining(t)
	r := shRun(t, "")
	program(t, filepath.Join(r.Home, "tools", "bin", "tool"), "tool-ran")
	creds := write(t, filepath.Join(r.Home, "tools", "credentials", "config"), "secret")
	require.NoError(t, os.Symlink(filepath.Dir(creds), filepath.Join(r.Home, ".kube")))
	r = withPath(r, filepath.Join(r.Home, "tools", "bin"))
	r.Args[1] = "tool; cat " + creds

	_, out := run(t, s, r)

	assert.Contains(t, out, "tool-ran")
	assert.NotContains(t, out, "secret")
}

// A root that is a link runs what is under its target, wherever that lies.
func TestARootThatIsALinkRuns(t *testing.T) {
	s := confining(t)
	base := resolved(t.TempDir())
	program(t, filepath.Join(base, "var", "opt", "bin", "tool"), "tool-ran")
	opt := filepath.Join(base, "opt")
	require.NoError(t, os.Symlink(filepath.Join(base, "var", "opt"), opt))
	addRoot(t, opt)

	code, out := run(t, s, withPath(shRun(t, "tool"), filepath.Join(opt, "bin")))

	assert.Equal(t, 0, code)
	assert.Equal(t, "tool-ran\n", out)
}

// A program in a root's PATH entry that links under the home runs, as
// /usr/local/bin/tool linked to ~/tools/tool does.
func TestAProgramLinkInARootRuns(t *testing.T) {
	s := confining(t)
	root := resolved(t.TempDir())
	addRoot(t, root)
	r := shRun(t, "tool")
	tool := program(t, filepath.Join(r.Home, "tools", "tool"), "tool-ran")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "bin"), 0o700))
	require.NoError(t, os.Symlink(tool, filepath.Join(root, "bin", "tool")))

	code, out := run(t, s, withPath(r, filepath.Join(root, "bin")))

	assert.Equal(t, 0, code)
	assert.Equal(t, "tool-ran\n", out)
}

// Kstack's directories inside a root are unreadable, but for the run's own
// workspace inside them.
func TestKstacksDirectoriesInsideARootAreUnreadable(t *testing.T) {
	s := confining(t)
	root := resolved(t.TempDir())
	addRoot(t, root)
	data := filepath.Join(root, "kstack")
	write(t, filepath.Join(data, "app.db"), "secret")
	ws := mkdirs(t, data, "chats/1/workspace")[0]
	r := shRun(t, "cat "+filepath.Join(data, "app.db")+"; touch w && echo wrote")
	r.Dir, r.Workspace = ws, ws
	r.Denied = []string{data}

	_, out := run(t, s, r)

	assert.Equal(t, "wrote\n", out)
	assert.FileExists(t, filepath.Join(ws, "w"))
}

// kstackRun is a run laid out as Kstack lays one out, with its data and cache
// directories where a PATH tree takes them in, and a sibling run beside it.
type kstackRun struct {
	Run
	data, cache, runtime, sibling string
}

func newKstackRun(t *testing.T) kstackRun {
	t.Helper()
	r := shRun(t, "")
	home := r.Home
	program(t, filepath.Join(home, "apps", "bin", "tool"), "tool-ran")
	program(t, filepath.Join(home, ".cache", "bin", "cached"), "cached-ran")
	data := filepath.Join(home, "apps", "kstack")
	cache := filepath.Join(home, ".cache", "kstack")
	runtime := resolved(t.TempDir())
	write(t, filepath.Join(data, "app.db"), "secret")
	write(t, filepath.Join(cache, "kubestore", "c.db"), "secret")
	write(t, filepath.Join(runtime, "host.sock"), "secret")
	sibling := write(t, filepath.Join(runtime, "runs", "2", "kubeconfig"), "secret")
	snapshot := write(t, filepath.Join(runtime, "shell", "snapshot.sh"), "snapshot")
	runDir := filepath.Dir(write(t, filepath.Join(runtime, "runs", "1", "kubeconfig"), "own-kubeconfig"))
	ws := mkdirs(t, data, "chats/1/workspace")[0]
	tmp := mkdirs(t, cache, "tmp/1")[0]
	kubectl := mkdirs(t, cache, "kubectl/c")[0]
	r.Dir, r.Workspace = ws, ws
	r.Readable = []string{snapshot, runDir}
	r.Writable = []string{tmp, kubectl}
	r.Denied = []string{data, cache, runtime}
	r = withPath(r, filepath.Join(home, "apps", "bin"), filepath.Join(home, ".cache", "bin"))
	return kstackRun{Run: r, data: data, cache: cache, runtime: runtime, sibling: sibling}
}

// Kstack's directories are unreadable, but for the run's own: its workspace,
// kubectl cache, snapshot, run directory and TMPDIR.
func TestKstacksDirectoriesAreUnreadable(t *testing.T) {
	s := confining(t)
	k := newKstackRun(t)
	k.Args[1] = fmt.Sprintf("tool; cached; cat %s/app.db %s/kubestore/c.db %s/host.sock; cat %s %s/kubeconfig; touch %s/w %s/w %s/w && echo wrote",
		k.data, k.cache, k.runtime, k.Readable[0], k.Readable[1], k.Workspace, k.Writable[0], k.Writable[1])

	_, out := run(t, s, k.Run)

	assert.Contains(t, out, "tool-ran\ncached-ran\n")
	assert.NotContains(t, out, "secret")
	assert.Contains(t, out, "snapshotown-kubeconfig")
	assert.Contains(t, out, "wrote")
}

func TestASiblingRunsKubeconfigIsUnreadable(t *testing.T) {
	s := confining(t)
	k := newKstackRun(t)
	k.Args[1] = "cat " + k.sibling + "; ls " + filepath.Dir(filepath.Dir(k.sibling))

	_, out := run(t, s, k.Run)

	assert.NotContains(t, out, "secret")
	assert.NotContains(t, out, "2\n")
}

// A write lands in the workspace, the cache or the run's own /tmp, and
// anywhere else fails; nothing outside the sandbox changes but the first two.
// The stand-in home is under the host's /tmp, which inside is the run's own,
// so a write there lands in it: it is checked outside alone.
func TestOnlyTheWorkspaceAndTheCacheAreWritten(t *testing.T) {
	s := confining(t)
	k := newKstackRun(t)
	marker := "kstack-write-" + strconv.Itoa(os.Getpid())
	var script strings.Builder
	for _, p := range []string{"/" + marker, "/etc/" + marker, "/usr/" + marker, k.data + "/" + marker, k.cache + "/" + marker} {
		fmt.Fprintf(&script, "touch %s 2>/dev/null && echo wrote %s; ", p, p)
	}
	fmt.Fprintf(&script, "touch %s/%s 2>/dev/null; ", k.Home, marker)
	fmt.Fprintf(&script, "touch /tmp/%s && echo tmp; touch %s/%s %s/%s && echo own", marker, k.Workspace, marker, k.Writable[1], marker)
	k.Args[1] = script.String()

	_, out := run(t, s, k.Run)

	assert.Equal(t, "tmp\nown\n", out)
	assert.NoFileExists(t, "/tmp/"+marker)
	assert.NoFileExists(t, filepath.Join(k.Home, marker))
	assert.NoFileExists(t, filepath.Join(k.data, marker))
	assert.FileExists(t, filepath.Join(k.Workspace, marker))
}

// A run cannot remove, rename or replace the root of a path it writes: each
// is a mount point, whose parent is the namespace's own. What lies inside
// stays its own.
func TestARunCannotReplaceItsWorkspace(t *testing.T) {
	s := confining(t)
	for _, script := range []string{
		`cd /; rm -rf "$W" && ln -s "$D" "$W"`,
		`cd /; rmdir "$W"`,
		`cd /; mv "$W" "$O/moved"`,
		`mkdir "$O/x" && mv -T "$O/x" "$W"`,
		`ln -s "$D" "$O/l" && mv -T "$O/l" "$W"`,
	} {
		k := newKstackRun(t)
		for _, pair := range [][2]string{{k.Workspace, k.Writable[1]}, {k.Writable[1], k.Workspace}} {
			w, other := pair[0], pair[1]
			r := k.Run
			r.Args = []string{"-c", script}
			r.Env = append(slices.Clone(r.Env), "W="+w, "D="+k.data, "O="+other)
			code, out := run(t, s, r)
			assert.NotEqual(t, 0, code, "%s on %s: %s", script, w, out)
			info, err := os.Lstat(w)
			require.NoError(t, err, script)
			assert.True(t, info.IsDir(), "%s on %s left %v", script, w, info.Mode())
		}
	}

	k := newKstackRun(t)
	for _, w := range []string{k.Workspace, k.Writable[1]} {
		r := k.Run
		r.Args = []string{"-c", `mkdir "$W/a" && echo x > "$W/a/f" && mv "$W/a" "$W/b" && rm -rf "$W/b" && ln -s /usr "$W/l" && rm "$W/l"`}
		r.Env = append(slices.Clone(r.Env), "W="+w)
		code, out := run(t, s, r)
		assert.Equal(t, 0, code, "%s: %s", w, out)
	}
}

func init() {
	helpers["dial"] = func() int {
		c, err := net.DialTimeout("tcp", os.Getenv("KSTACK_SANDBOX_TEST_ADDR"), time.Second)
		if err != nil {
			fmt.Println(err)
			return 1
		}
		_ = c.Close()
		fmt.Println("connected")
		return 0
	}
}

// self is a run of this test binary as the helper named, with env.
func self(t *testing.T, helper string, env ...string) Run {
	t.Helper()
	r := shRun(t, "")
	r.Shell, r.Args = os.Args[0], nil
	r.Env = append(append(r.Env, "KSTACK_SANDBOX_TEST_HELPER="+helper), env...)
	return r
}

func TestNoListenerOutsideIsReachable(t *testing.T) {
	s := confining(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	code, out := run(t, s, self(t, "dial", "KSTACK_SANDBOX_TEST_ADDR="+ln.Addr().String()))

	assert.Equal(t, 1, code)
	assert.Contains(t, out, "connection refused")
}

// A lookup fails, and fails at once rather than waiting on a resolver.
func TestAPublicNameDoesNotResolve(t *testing.T) {
	s := confining(t)
	start := time.Now()

	code, out := run(t, s, shRun(t, "getent hosts example.com"))

	assert.NotEqual(t, 0, code, out)
	assert.Less(t, time.Since(start), time.Second)
}

// The shell is refused a Unix or vsock socket while the forwarder, outside
// the filter, still relays a connection to the run's.
func TestAUnixOrVsockSocketCannotBeOpened(t *testing.T) {
	s := confining(t)
	socket := echoSocket(t)
	r := self(t, "sockets")
	r.Readable = []string{filepath.Dir(socket)}
	r.Socket, r.Port = socket, forwarderPort

	_, out := run(t, s, r)
	assert.Equal(t, "unix=operation not permitted vsock=operation not permitted inet=ok dgram-pair=operation not permitted stream-pair=ok", out)

	r = self(t, "", "KSTACK_SANDBOX_TEST_DIAL="+strconv.Itoa(forwarderPort))
	r.Readable = []string{filepath.Dir(socket)}
	r.Socket, r.Port = socket, forwarderPort
	code, out := run(t, s, r)
	assert.Equal(t, 0, code)
	assert.Equal(t, "ping|eof", out)
}

// A datagram pair cannot be made, so nothing under the filter sends to a
// host's socket in a directory it can read.
func TestNoDatagramReachesAHostSocket(t *testing.T) {
	s := confining(t)
	dir := resolved(t.TempDir())
	path := filepath.Join(dir, "host.sock")
	ln, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	require.NoError(t, err)
	defer ln.Close()
	r := self(t, "sendto", "KSTACK_SANDBOX_TEST_SOCKET="+path)
	r.Readable = []string{dir}

	code, out := run(t, s, r)

	assert.Equal(t, 1, code)
	assert.Equal(t, "operation not permitted", out)
	// The run has ended, so anything it sent is queued: a read that waits
	// for nothing finds it.
	require.NoError(t, ln.SetReadDeadline(time.Now()))
	_, _, err = ln.ReadFrom(make([]byte, 16))
	assert.ErrorIs(t, err, os.ErrDeadlineExceeded)
}

func TestAUserNamespaceCannotBeMade(t *testing.T) {
	s := confining(t)

	code, _ := run(t, s, shRun(t, "unshare -U true"))

	assert.NotEqual(t, 0, code)
}

// The forwarder is the namespace's first process, and nothing under the
// filter can read its environment or memory.
func TestTheFirstProcessCannotBeRead(t *testing.T) {
	s := confining(t)
	r := shRun(t, "tr '\\0' ' ' < /proc/1/cmdline; echo; cat /proc/1/environ >/dev/null 2>&1 || echo environ-refused; head -c1 /proc/1/mem >/dev/null 2>&1 || echo mem-refused")

	_, out := run(t, s, r)

	assert.Contains(t, out, InitCommand)
	assert.Contains(t, out, "environ-refused\nmem-refused\n")
}

// A command that leaves a process behind ends at once whether its group is
// sent SIGTERM or SIGKILL, and nothing of the run is left.
func TestNothingOutlivesTheGroupsKill(t *testing.T) {
	s := confining(t)
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL} {
		marker := fmt.Sprintf("3001.%d%d", os.Getpid(), sig)
		cmd := s.Command(t.Context(), shRun(t, "sleep "+marker+" & echo up; exec sleep 3002"))
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		out, err := cmd.StdoutPipe()
		require.NoError(t, err)
		require.NoError(t, cmd.Start())
		line, err := bufio.NewReader(out).ReadString('\n')
		require.NoError(t, err)
		require.Equal(t, "up\n", line)

		require.NoError(t, syscall.Kill(-cmd.Process.Pid, sig))
		_ = cmd.Wait()

		require.Eventually(t, func() bool { return !running(marker) }, 5*time.Second, 10*time.Millisecond, "%s left %s running", sig, marker)
	}
}

// running reports whether a process whose command line holds marker is alive.
func running(marker string) bool {
	procs, _ := filepath.Glob("/proc/[0-9]*/cmdline")
	for _, p := range procs {
		if b, err := os.ReadFile(p); err == nil && strings.Contains(string(b), marker) {
			return true
		}
	}
	return false
}

// A process orphaned under the forwarder is reaped: its /proc entry goes,
// where an unreaped one would stay a zombie and hold the loop open.
func TestAnOrphanIsReaped(t *testing.T) {
	s := confining(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	r := shRun(t, `pid=$(sh -c 'true & echo $!'); while [ -e /proc/$pid ]; do :; done; echo reaped`)

	cmd := s.Command(ctx, r)
	out, err := cmd.Output()

	require.NoError(t, err)
	assert.Equal(t, "reaped\n", string(out))
}
