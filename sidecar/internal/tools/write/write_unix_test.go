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

package write

import (
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kstackhq/kstack/sidecar/internal/tools"
)

// Under the sidecar's owner-only umask, a new file is 0o666 and each directory
// made for it 0o777, less the umask the tool was given.
//
// syscall.Umask is process-global: no t.Parallel() here.
func TestWriteCreatesAFileAndItsParents(t *testing.T) {
	prev := syscall.Umask(0o077)
	defer syscall.Umask(prev)
	tl, rt, _, _ := chat(t)
	root := t.TempDir()
	path := filepath.Join(root, "a", "b", "x.txt")

	_, isError := tl.Run(t.Context(), rt, call(path, "x\n"))
	require.False(t, isError)
	for p, want := range map[string]os.FileMode{
		path:                          0o644,
		filepath.Join(root, "a"):      os.ModeDir | 0o755,
		filepath.Join(root, "a", "b"): os.ModeDir | 0o755,
	} {
		info, err := os.Lstat(p)
		require.NoError(t, err)
		assert.Equal(t, want, info.Mode(), p)
	}
}

// An existing file keeps its permission bits, never setuid, and its group.
func TestWriteKeepsAnExistingModeAndGroup(t *testing.T) {
	tl, rt, st, _ := chat(t)
	for _, mode := range []os.FileMode{0o600, 0o755} {
		path := file(t, st, "a\n")
		require.NoError(t, os.Chmod(path, mode|os.ModeSetuid))
		_, isError := tl.Run(t.Context(), rt, call(path, "b\n"))
		require.False(t, isError)
		info, err := os.Lstat(path)
		require.NoError(t, err)
		assert.Equal(t, mode, info.Mode())
	}

	path := file(t, st, "a\n")
	info, err := os.Lstat(path)
	require.NoError(t, err)
	gid := int(info.Sys().(*syscall.Stat_t).Gid)
	groups, err := os.Getgroups()
	require.NoError(t, err)
	for _, g := range groups {
		if g == gid {
			continue
		}
		require.NoError(t, os.Chown(path, -1, g))
		_, isError := tl.Run(t.Context(), rt, call(path, "b\n"))
		require.False(t, isError)
		info, err := os.Lstat(path)
		require.NoError(t, err)
		assert.Equal(t, g, int(info.Sys().(*syscall.Stat_t).Gid))
		return
	}
	// fileguard's TestReplaceableTakesAGroupTheNewFileGets decides the group
	// against a stubbed list, so the case runs where this part cannot.
	t.Log("the test user is in one group; the group is checked in fileguard")
}

// A link in the last component is asked about like any path, since the gate
// reads the name alone, then refused with its target; nothing is written
// through it.
func TestWriteRefusesALinkNamingItsTarget(t *testing.T) {
	tl, rt, st, _ := chat(t)
	target := file(t, st, "a\n")
	link := filepath.Join(t.TempDir(), "l")
	require.NoError(t, os.Symlink(target, link))
	st[link] = st[target]

	assert.Equal(t, tools.Approval{}, approval(t, tl, rt, call(link, "b\n")))
	text, isError := tl.Run(t.Context(), rt, call(link, "b\n"))
	assert.True(t, isError)
	assert.Equal(t, "This is a symbolic link. Write to its target instead: "+target, text)
	assert.Equal(t, "a\n", content(t, target))
}

// A file the user cannot write, one in a directory they cannot write, and a new
// file under a directory they cannot write are refused before anything
// changes.
func TestWriteRefusesWhatItCannotWrite(t *testing.T) {
	tl, rt, st, _ := chat(t)
	ro := file(t, st, "a\n")
	require.NoError(t, os.Chmod(ro, 0o444))

	inShut := file(t, st, "a\n")
	shut := filepath.Dir(inShut)
	require.NoError(t, os.Chmod(shut, 0o500))
	t.Cleanup(func() { _ = os.Chmod(shut, 0o700) })

	for path, want := range map[string]string{
		ro:                                  "Kstack cannot write this file.",
		inShut:                              "Kstack cannot replace files in this directory.",
		filepath.Join(shut, "new", "x.txt"): "Kstack cannot write in " + shut + ".",
	} {
		text, isError := tl.Run(t.Context(), rt, call(path, "b\n"))
		assert.True(t, isError, path)
		assert.Equal(t, want, text, path)
	}
	assert.Equal(t, "a\n", content(t, ro))
	assert.Equal(t, []string{"x.txt"}, names(t, shut))
}

// A file the user can write but not read is refused, since its stamp cannot be
// checked; a path whose ancestor cannot be resolved is refused, since the
// fence cannot tell.
func TestWriteRefusesWhatItCannotRead(t *testing.T) {
	tl, rt, st, _ := chat(t)
	path := file(t, st, "a\n")
	require.NoError(t, os.Chmod(path, 0o200))
	text, isError := tl.Run(t.Context(), rt, call(path, "b\n"))
	assert.True(t, isError)
	assert.Equal(t, "Kstack cannot read this file.", text)

	locked := filepath.Join(t.TempDir(), "locked")
	require.NoError(t, os.Mkdir(locked, 0))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	text, isError = tl.Run(t.Context(), rt, call(filepath.Join(locked, "x.txt"), "b\n"))
	assert.True(t, isError)
	assert.Equal(t, "Kstack cannot write this file.", text)
}

// The data directory reached through a link in a parent is refused on disk,
// with no directory made under it.
func TestWriteRefusesTheDataDirThroughALink(t *testing.T) {
	tl, rt, _, data := chat(t)
	link := filepath.Join(t.TempDir(), "l")
	require.NoError(t, os.Symlink(data, link))

	text, isError := tl.Run(t.Context(), rt, call(filepath.Join(link, "new", "x.txt"), "x"))
	assert.True(t, isError)
	assert.Equal(t, "Write cannot change Kstack's directories.", text)
	assert.Empty(t, names(t, data))
}

// A link in the workspace is refused rather than followed, at the file and at a
// directory on the way, and nothing outside changes.
func TestALinkOutOfTheWorkspaceIsRefused(t *testing.T) {
	tl, rt, _, _ := chat(t)
	ws := tools.WorkspacePath(rt.Dir)
	require.NoError(t, os.MkdirAll(ws, 0o700))
	outside := t.TempDir()
	target := filepath.Join(outside, "zshrc")
	require.NoError(t, os.WriteFile(target, []byte("keep\n"), 0o600))
	require.NoError(t, os.Symlink(target, filepath.Join(ws, "notes")))
	require.NoError(t, os.Symlink(outside, filepath.Join(ws, "dir")))

	text, isError := tl.Run(t.Context(), rt, call(filepath.Join(ws, "notes"), "x"))
	assert.True(t, isError)
	assert.Equal(t, "This is a symbolic link. Write to its target instead: "+target, text)

	for _, path := range []string{filepath.Join(ws, "dir", "x.txt"), filepath.Join(ws, "dir", "a", "x.txt")} {
		_, isError = tl.Run(t.Context(), rt, call(path, "x"))
		assert.True(t, isError, path)
	}
	assert.Equal(t, []string{"zshrc"}, names(t, outside))
	b, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "keep\n", string(b))
}

// Every path the gate lets through unasked is written through the workspace's
// root: a link in the workspace leading out is skipped by name and then
// refused, and a plain name is written under the root.
func TestAWriteThatSkipsGoesThroughTheRoot(t *testing.T) {
	tl, rt, _, _ := chat(t)
	ws := tools.WorkspacePath(rt.Dir)
	require.NoError(t, os.MkdirAll(ws, 0o700))
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(ws, "out")))

	escape := filepath.Join(ws, "out", "x.txt")
	assert.Equal(t, tools.Approval{Skip: true}, approval(t, tl, rt, call(escape, "x")))
	_, isError := tl.Run(t.Context(), rt, call(escape, "x"))
	assert.True(t, isError)
	assert.Empty(t, names(t, outside))

	plain := filepath.Join(ws, "x.txt")
	assert.Equal(t, tools.Approval{Skip: true}, approval(t, tl, rt, call(plain, "x")))
	text, isError := tl.Run(t.Context(), rt, call(plain, "x"))
	assert.False(t, isError, text)
	assert.Equal(t, "x", content(t, plain))
}

// A replace is a rename: a reader holding the old file reads the old bytes,
// and no temporary file is left.
func TestWriteIsARename(t *testing.T) {
	tl, rt, st, _ := chat(t)
	path := file(t, st, "old\n")
	held, err := os.Open(path)
	require.NoError(t, err)
	defer held.Close()

	_, isError := tl.Run(t.Context(), rt, call(path, "new\n"))
	require.False(t, isError)
	b, err := io.ReadAll(held)
	require.NoError(t, err)
	assert.Equal(t, "old\n", string(b))
	assert.Equal(t, []string{"x.txt"}, names(t, filepath.Dir(path)))
}
