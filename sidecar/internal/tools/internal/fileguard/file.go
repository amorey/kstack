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

package fileguard

import (
	"context"
	"errors"
	"io/fs"
	"os"

	"github.com/kstackhq/kstack/sidecar/internal/tools"
)

// ErrFenced is a path under Kstack's directories, and outside the chat's
// workspace.
var ErrFenced = errors.New("files: under Kstack's directories")

// NamedOutsideWorkspace reports whether path is under the fence by name and
// not under the chat's workspace, the fence's one opening. It touches nothing
// on disk.
func (f Fence) NamedOutsideWorkspace(dir tools.ChatDir, path string) bool {
	_, inWorkspace := Under(tools.WorkspacePath(dir), path)
	return f.Named(path) && !inWorkspace
}

// File is a file a tool changes: a name under the chat's workspace, reached
// through the workspace's root, or a path outside Kstack's directories.
type File struct {
	path string
	root *os.Root // nil for a path outside the workspace
	name string   // under root
}

// File is path as a tool changes it. A path under the workspace by name opens
// the workspace, made first when create is set; a missing one is ErrMissing. A
// path that reaches the workspace by another spelling is not under it by name,
// so the fence's check on disk refuses it with the rest of Kstack's directories
// (ErrFenced). The caller closes the File.
func (f Fence) File(dir tools.ChatDir, path string, create bool) (File, error) {
	if name, ok := Under(tools.WorkspacePath(dir), path); ok {
		root, err := tools.OpenWorkspace(dir, create)
		if errors.Is(err, fs.ErrNotExist) {
			return File{}, ErrMissing
		}
		if err != nil {
			return File{}, err
		}
		return File{path: path, root: root, name: name}, nil
	}
	held, err := f.Holds(path)
	if err != nil {
		return File{}, err
	}
	if held {
		return File{}, ErrFenced
	}
	return File{path: path}, nil
}

// Close closes the workspace's root, if the File holds one.
func (f File) Close() error {
	if f.root == nil {
		return nil
	}
	return f.root.Close()
}

// Lstat is Lstat or LstatIn.
func (f File) Lstat() (os.FileInfo, error) {
	if f.root == nil {
		return Lstat(f.path)
	}
	return LstatIn(f.root, f.name)
}

// Open is Open or OpenIn.
func (f File) Open() (*os.File, error) {
	if f.root == nil {
		return Open(f.path)
	}
	return OpenIn(f.root, f.name)
}

// Replaceable is Replaceable or ReplaceableIn.
func (f File) Replaceable(info os.FileInfo) error {
	if f.root == nil {
		return Replaceable(f.path, info)
	}
	return ReplaceableIn(f.root, f.name, info)
}

// Replace is Replace or ReplaceIn.
func (f File) Replace(ctx context.Context, content []byte, old os.FileInfo) error {
	if f.root == nil {
		return Replace(ctx, f.path, content, old)
	}
	return ReplaceIn(ctx, f.root, f.name, content, old)
}

// Creatable is Creatable or CreatableIn.
func (f File) Creatable() error {
	if f.root == nil {
		return Creatable(f.path)
	}
	return CreatableIn(f.root, f.name)
}

// Create is Create or CreateIn.
func (f File) Create(ctx context.Context, content []byte, umask fs.FileMode) error {
	if f.root == nil {
		return Create(ctx, f.path, content, umask)
	}
	return CreateIn(ctx, f.root, f.name, content, umask)
}
