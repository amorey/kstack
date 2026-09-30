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

package sandbox

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The command follows a --, and there must be one.
func TestParseShellArgs(t *testing.T) {
	got, err := parseShellArgs([]string{"--", "/bin/sh", "-c", "true"})
	require.NoError(t, err)
	assert.Equal(t, []string{"/bin/sh", "-c", "true"}, got)
	for _, args := range [][]string{nil, {"--"}, {"/bin/sh"}} {
		_, err := parseShellArgs(args)
		assert.Error(t, err, args)
	}
}
