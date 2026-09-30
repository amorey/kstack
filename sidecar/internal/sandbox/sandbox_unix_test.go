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

package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A run's own path that is a link is refused, whichever list it is in, since
// the profile would open where the link leads. A link above it is not.
func TestARunsOwnPathThatIsALinkIsRefused(t *testing.T) {
	base := t.TempDir()
	d := mkdirs(t, base, "data/chats/c/workspace", "cache/tmp/1-a", "runtime/runs/1-a", "real")
	link := filepath.Join(base, "data/chats/c/linked")
	require.NoError(t, os.Symlink(filepath.Join(base, "data"), link))
	above := filepath.Join(base, "above")
	require.NoError(t, os.Symlink(d[3], above))
	require.NoError(t, os.Mkdir(filepath.Join(d[3], "ws"), 0o700))
	run := Run{Workspace: d[0], Writable: []string{d[1]}, Readable: []string{d[2]}}
	require.NoError(t, run.Check())

	for name, r := range map[string]Run{
		"workspace": {Workspace: link, Writable: run.Writable, Readable: run.Readable},
		"writable":  {Workspace: d[0], Writable: []string{d[1], link}, Readable: run.Readable},
		"readable":  {Workspace: d[0], Writable: run.Writable, Readable: []string{link}},
	} {
		assert.ErrorContains(t, r.Check(), "is a link", name)
	}
	assert.NoError(t, Run{Workspace: filepath.Join(above, "ws")}.Check(), "a link above the last component")
	assert.NoError(t, Run{Workspace: filepath.Join(base, "missing")}.Check(), "a path that is not there")
}
