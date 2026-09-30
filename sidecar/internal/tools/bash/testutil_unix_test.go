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

//go:build !windows

package bash

import (
	"testing"

	"github.com/kstackhq/kstack/sidecar/internal/sandbox"
	"github.com/kstackhq/kstack/sidecar/internal/testutil"
)

// sandboxed is this machine's sandbox, for a test that runs a command through
// it, or testutil.RequireSandbox's verdict where there is none.
func sandboxed(t *testing.T) *sandbox.Sandbox {
	t.Helper()
	s, v := sandbox.Probe(t.Context())
	if v.Available {
		return s
	}
	testutil.RequireSandbox(t, "no sandbox: "+v.Reason)
	return nil
}

// confining is this machine's sandbox, for a test that needs it to confine,
// or testutil.RequireSandbox's verdict where there is none.
func confining(t *testing.T) *sandbox.Sandbox {
	t.Helper()
	s, v := sandbox.Probe(t.Context())
	if s != nil && s.Confines() {
		return s
	}
	testutil.RequireSandbox(t, "no confining sandbox: "+v.Reason)
	return nil
}
