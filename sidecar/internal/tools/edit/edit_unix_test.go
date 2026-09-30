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

package edit

import (
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kstackhq/kstack/sidecar/internal/tools"
)

// An edited file keeps its permission bits, never setuid, and its group.
func TestEditKeepsTheModeAndGroup(t *testing.T) {
	tl, rt, st, _ := chat(t)
	for _, mode := range []os.FileMode{0o600, 0o755} {
		path := file(t, st, "a\n")
		require.NoError(t, os.Chmod(path, mode|os.ModeSetuid))
		_, isError := tl.Run(t.Context(), rt, call(path, "a", "b"))
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
		_, isError := tl.Run(t.Context(), rt, call(path, "a", "b"))
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
// reads the name alone, then refused with its target; nothing is changed
// through it.
func TestEditRefusesALinkNamingItsTarget(t *testing.T) {
	tl, rt, st, _ := chat(t)
	target := file(t, st, "a\n")
	link := filepath.Join(t.TempDir(), "l")
	require.NoError(t, os.Symlink(target, link))
	st[link] = st[target]

	assert.Equal(t, tools.Approval{}, approval(t, tl, rt, call(link, "a", "b")))
	text, isError := tl.Run(t.Context(), rt, call(link, "a", "b"))
	assert.True(t, isError)
	assert.Equal(t, "This is a symbolic link. Edit its target instead: "+target, text)
	assert.Equal(t, "a\n", content(t, target))
}

// A file the user cannot write, and one in a directory they cannot write, are
// refused before anything changes.
func TestEditRefusesWhatItCannotWrite(t *testing.T) {
	tl, rt, st, _ := chat(t)
	ro := file(t, st, "a\n")
	require.NoError(t, os.Chmod(ro, 0o444))

	inShut := file(t, st, "a\n")
	shut := filepath.Dir(inShut)
	require.NoError(t, os.Chmod(shut, 0o500))
	t.Cleanup(func() { _ = os.Chmod(shut, 0o700) })

	for path, want := range map[string]string{
		ro:     "Kstack cannot write this file.",
		inShut: "Kstack cannot replace files in this directory.",
	} {
		text, isError := tl.Run(t.Context(), rt, call(path, "a", "b"))
		assert.True(t, isError, path)
		assert.Equal(t, want, text, path)
		assert.Equal(t, "a\n", content(t, path), path)
	}
	assert.Equal(t, []string{"x.txt"}, names(t, shut))
}

// A file the user can write but not read is refused, since its stamp cannot be
// checked; a path whose ancestor cannot be resolved is refused, since the
// fence cannot tell.
func TestEditRefusesWhatItCannotRead(t *testing.T) {
	tl, rt, st, _ := chat(t)
	path := file(t, st, "a\n")
	require.NoError(t, os.Chmod(path, 0o200))
	text, isError := tl.Run(t.Context(), rt, call(path, "a", "b"))
	assert.True(t, isError)
	assert.Equal(t, "Kstack cannot read this file.", text)

	locked := filepath.Join(t.TempDir(), "locked")
	require.NoError(t, os.Mkdir(locked, 0))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	text, isError = tl.Run(t.Context(), rt, call(filepath.Join(locked, "x.txt"), "a", "b"))
	assert.True(t, isError)
	assert.Equal(t, "Kstack cannot write this file.", text)
}

// The data directory reached through a link in a parent is refused on disk.
func TestEditRefusesTheDataDirThroughALink(t *testing.T) {
	tl, rt, st, data := chat(t)
	inData := filepath.Join(data, "host.json")
	require.NoError(t, os.WriteFile(inData, []byte("a\n"), 0o600))
	link := filepath.Join(t.TempDir(), "l")
	require.NoError(t, os.Symlink(data, link))
	through := filepath.Join(link, "host.json")
	st[through] = st[file(t, st, "a\n")]

	text, isError := tl.Run(t.Context(), rt, call(through, "a", "b"))
	assert.True(t, isError)
	assert.Equal(t, "Edit cannot change Kstack's directories.", text)
	assert.Equal(t, "a\n", content(t, inData))
}

// A link in the workspace is refused rather than followed, at the file and at a
// directory on the way, and nothing outside changes.
func TestALinkOutOfTheWorkspaceIsRefused(t *testing.T) {
	tl, rt, st, _ := chat(t)
	ws := tools.WorkspacePath(rt.Dir)
	require.NoError(t, os.MkdirAll(ws, 0o700))
	outside := t.TempDir()
	target := filepath.Join(outside, "zshrc")
	require.NoError(t, os.WriteFile(target, []byte("keep\n"), 0o600))
	require.NoError(t, os.Symlink(target, filepath.Join(ws, "notes")))
	require.NoError(t, os.Symlink(outside, filepath.Join(ws, "dir")))
	for _, path := range []string{filepath.Join(ws, "notes"), filepath.Join(ws, "dir", "zshrc")} {
		st[path] = tools.Stamp{Sum: sha256.Sum256([]byte("keep\n")), Whole: true}
	}

	text, isError := tl.Run(t.Context(), rt, call(filepath.Join(ws, "notes"), "keep", "x"))
	assert.True(t, isError)
	assert.Equal(t, "This is a symbolic link. Edit its target instead: "+target, text)
	_, isError = tl.Run(t.Context(), rt, call(filepath.Join(ws, "dir", "zshrc"), "keep", "x"))
	assert.True(t, isError)

	b, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "keep\n", string(b))
}

// Every path the gate lets through unasked is changed through the workspace's
// root: a link in the workspace leading out is skipped by name and then
// refused, and a plain name is changed under the root.
func TestAWriteThatSkipsGoesThroughTheRoot(t *testing.T) {
	tl, rt, st, _ := chat(t)
	ws := tools.WorkspacePath(rt.Dir)
	require.NoError(t, os.MkdirAll(ws, 0o700))
	outside := t.TempDir()
	target := filepath.Join(outside, "x.txt")
	require.NoError(t, os.WriteFile(target, []byte("keep\n"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(ws, "out")))
	escape := filepath.Join(ws, "out", "x.txt")
	plain := filepath.Join(ws, "x.txt")
	require.NoError(t, os.WriteFile(plain, []byte("keep\n"), 0o600))
	for _, path := range []string{escape, plain} {
		st[path] = tools.Stamp{Sum: sha256.Sum256([]byte("keep\n")), Whole: true}
	}

	assert.Equal(t, tools.Approval{Skip: true}, approval(t, tl, rt, call(escape, "keep", "x")))
	_, isError := tl.Run(t.Context(), rt, call(escape, "keep", "x"))
	assert.True(t, isError)
	assert.Equal(t, "keep\n", content(t, target))

	assert.Equal(t, tools.Approval{Skip: true}, approval(t, tl, rt, call(plain, "keep", "x")))
	text, isError := tl.Run(t.Context(), rt, call(plain, "keep", "x"))
	assert.False(t, isError, text)
	assert.Equal(t, "x\n", content(t, plain))
}

// An edit is a rename: a reader holding the old file reads the old bytes, and
// no temporary file is left.
func TestEditIsARename(t *testing.T) {
	tl, rt, st, _ := chat(t)
	path := file(t, st, "old\n")
	held, err := os.Open(path)
	require.NoError(t, err)
	defer held.Close()

	_, isError := tl.Run(t.Context(), rt, call(path, "old", "new"))
	require.False(t, isError)
	b, err := io.ReadAll(held)
	require.NoError(t, err)
	assert.Equal(t, "old\n", string(b))
	assert.Equal(t, []string{"x.txt"}, names(t, filepath.Dir(path)))
}
