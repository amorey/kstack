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

package read

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kstackhq/kstack/sidecar/internal/testutil"
	"github.com/kstackhq/kstack/sidecar/internal/tools"
)

// A FIFO in the directory is refused at once, with no writer: opening one for
// reading would otherwise block past any cancel.
func TestReadRefusesAFIFOAtOnce(t *testing.T) {
	dir, tl, rt := chat(t)
	fifo := filepath.Join(dir, "fifo")
	require.NoError(t, syscall.Mkfifo(fifo, 0o600))

	var text string
	var isError bool
	testutil.WaitReturn(t, func() { text, isError = tl.Run(t.Context(), rt, call(fifo)) }, "the FIFO's refusal")
	assert.True(t, isError)
	assert.Equal(t, notHere, text)
}

// The data directory is refused however it is reached: a link elsewhere into
// it, or a link to one of its files, is notHere, and the refusal names no
// target.
func TestReadRefusesTheDataDirThroughALink(t *testing.T) {
	dir, tl, rt := chat(t)
	data := filepath.Dir(filepath.Dir(dir))
	require.NoError(t, os.WriteFile(filepath.Join(data, "app.db"), []byte("rows"), 0o600))
	elsewhere := t.TempDir()
	require.NoError(t, os.Symlink(data, filepath.Join(elsewhere, "d")))
	require.NoError(t, os.Symlink(filepath.Join(data, "app.db"), filepath.Join(elsewhere, "db")))

	for _, path := range []string{
		filepath.Join(elsewhere, "d", "app.db"),
		filepath.Join(elsewhere, "db"),
		filepath.Join(elsewhere, "d", "chats", "c2", "x.txt"),
	} {
		assert.Equal(t, tools.Approval{}, approval(t, tl, rt, path), "by name it is elsewhere, so it is asked")
		text, isError := tl.Run(t.Context(), rt, call(path))
		assert.True(t, isError, path)
		assert.Equal(t, notHere, text, path)
	}
}

// The refusals only Unix can stage: a link names its target, a FIFO is not a
// regular file, and a file the sidecar cannot read, or whose place it cannot
// tell, is one sentence either way.
func TestReadRefusalsSayWhatIsWrongOnUnix(t *testing.T) {
	_, tl, rt := chat(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "x.yaml"), []byte("x"), 0o600))
	require.NoError(t, os.Symlink("x.yaml", filepath.Join(dir, "link")))
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "fifo"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "locked.txt"), []byte("x"), 0))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "shut"), 0))
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(dir, "shut"), 0o700) })

	for path, want := range map[string]string{
		filepath.Join(dir, "link"):              "This is a symbolic link. Read its target instead: " + filepath.Join(dir, "x.yaml"),
		filepath.Join(dir, "fifo"):              "This is not a regular file.",
		filepath.Join(dir, "locked.txt"):        "Kstack cannot read this file.",
		filepath.Join(dir, "shut", "inner.txt"): "Kstack cannot read this file.",
	} {
		text, isError := tl.Run(t.Context(), rt, call(path))
		assert.True(t, isError, path)
		assert.Equal(t, want, text, path)
	}
}

// A link in the directory that leads out of it is refused at the open.
func TestReadRefusesALinkOut(t *testing.T) {
	dir, tl, rt := chat(t)
	target := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(target, []byte("secret"), 0o600))
	require.NoError(t, os.Symlink(target, filepath.Join(dir, "link.txt")))

	text, isError := tl.Run(t.Context(), rt, call(filepath.Join(dir, "link.txt")))
	assert.True(t, isError)
	assert.Equal(t, notHere, text)
}
