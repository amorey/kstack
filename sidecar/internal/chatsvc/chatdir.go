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

// Each chat's own directory: its saved results, its background tasks' output and
// its workspace, under the chats' directory and deleted with the chat.
package chatsvc

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/kstackhq/kstack/sidecar/internal/rootdir"
	"github.com/kstackhq/kstack/sidecar/internal/tools"
)

// openChats makes the chats' directory owner-only and opens it as the root
// every chat's directory is reached through. dir is absolute: a result names
// its file under the root's name, and Read takes only an absolute path.
func openChats(dir string) (*os.Root, error) {
	root, err := rootdir.MakeRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open the chats' directory: %w", err)
	}
	return root, nil
}

// chatDir is one chat's entry in the chats' directory.
type chatDir struct {
	s  *service
	id ChatID
}

var _ tools.ChatDir = chatDir{}

func (s *service) chatDir(id ChatID) chatDir { return chatDir{s: s, id: id} }

func (d chatDir) Path() string { return filepath.Join(d.s.chatsRoot.Name(), string(d.id)) }

// Root reaches the chat's entry through the chats' root, so a link swapped in
// for it is refused rather than followed.
func (d chatDir) Root(create bool) (*os.Root, error) {
	return rootdir.Open(d.s.chatsRoot, string(d.id), create)
}

// removeChatDir removes the chat's entry, a link rather than its target. An
// entry already gone is a removal done. A failure is logged, and the next
// start sweeps what it left.
func (s *service) removeChatDir(id ChatID) {
	if err := rootdir.RemoveAll(s.chatsRoot, string(id)); err != nil {
		slog.Warn("could not remove a chat's directory; the next start sweeps it", "chat", id, "err", err)
	}
}
