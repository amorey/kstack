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

package chatsvc

import (
	"encoding/json"

	"github.com/kstackhq/kstack/sidecar/internal/clustercard"
	"github.com/kstackhq/kstack/sidecar/internal/tools"
)

// workspaceShare is the Workspace section's share of the context block: a
// path and its heading, an estimate, so a longer path still goes.
const workspaceShare = 512

// withWorkspace is card with the chat's workspace appended as its own section:
// the file tools take absolute paths alone, so the model learns the path here.
// The path is fixed for the chat, so the section changes the context block on
// the chat's first send alone.
func (s *service) withWorkspace(card string, id ChatID) string {
	raw, err := json.Marshal(map[string]string{"path": tools.WorkspacePath(s.chatDir(id))})
	if err == nil {
		card, err = clustercard.WithSection(card, "Workspace", raw)
	}
	if err != nil {
		// A string map always marshals, and WithSection takes any one value.
		panic(err)
	}
	return card
}
