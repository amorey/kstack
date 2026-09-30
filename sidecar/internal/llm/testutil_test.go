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

package llm

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

type event struct{ event, data string }

// searchName is the test search's name: what its calls and usage are recorded
// under. The wire reads a call by its call name, never this.
const searchName = "acme_search"

// sse writes one event the way the API does, flushing so a test reads it as it lands.
func sse(w http.ResponseWriter, e event) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.event, e.data)
	w.(http.Flusher).Flush()
}

// recorded is what the server was asked, for the request half of a test.
type recorded struct {
	calls  atomic.Int64
	mu     sync.Mutex
	body   map[string]any
	header http.Header
	path   string
	query  url.Values
}

func (r *recorded) take(t *testing.T, req *http.Request) {
	t.Helper()
	r.calls.Add(1)
	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(body, &decoded))
	r.mu.Lock()
	defer r.mu.Unlock()
	r.body, r.header, r.path, r.query = decoded, req.Header.Clone(), req.URL.Path, req.URL.Query()
}

// newServer runs handler until the test ends and answers its base URL.
func newServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv.URL
}

// newRawServer answers one connection with line as its whole status line, and
// hands back its base URL. The request is read before the reply goes: closing a
// socket with bytes still unread sends a RST, which would lose the reply the
// test is about and leave a transport failure in its place.
func newRawServer(t *testing.T, line string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		br := bufio.NewReader(c)
		if req, err := http.ReadRequest(br); err == nil {
			io.Copy(io.Discard, req.Body)
		}
		fmt.Fprintf(c, "HTTP/1.1 %s\r\n\r\n", line)
	}()
	return "http://" + ln.Addr().String()
}

// openAIEnv is the SDK's whole chain at v3.61.0, read off DefaultClientOptions.
// It moves with a bump.
var openAIEnv = []string{
	"OPENAI_BASE_URL", "OPENAI_API_KEY", "OPENAI_ADMIN_KEY", "OPENAI_ORG_ID",
	"OPENAI_PROJECT_ID", "OPENAI_WEBHOOK_SECRET", "OPENAI_CUSTOM_HEADERS",
}
