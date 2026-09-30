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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kstackhq/kstack/sidecar/internal/llm"
	"github.com/kstackhq/kstack/sidecar/internal/tools"
)

// A chat's first question tells the model its workspace's path, after the
// card, and the path is fixed for the chat, so the next question with nothing
// else changed carries no context block.
func TestTheContextNamesTheWorkspace(t *testing.T) {
	s := serviceWithClusterCards(t, &stubClusterCards{card: "## Cluster\n\n```json\n{}\n```"})

	first := sendAndSettle(t, s, nil, "1", "1", "one")
	path, err := json.Marshal(tools.WorkspacePath(s.chatDir(first.ChatID)))
	require.NoError(t, err)
	context := newestContextOf(t, s, first.ChatID)
	assert.Contains(t, context, "## Cluster")
	assert.Contains(t, context, "## Workspace\n\n```json\n{\"path\":"+string(path)+"}\n```")

	sendAndSettle(t, s, &first.ChatID, "1", "2", "two")
	assert.Equal(t, []llm.BlockType{llm.BlockText}, blockTypes(t, questions(t, s, first.ChatID)[1]), "nothing changed")
}
