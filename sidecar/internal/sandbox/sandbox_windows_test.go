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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWindowsHasNoSandbox(t *testing.T) {
	s, v := Probe(context.Background())
	assert.Nil(t, s)
	assert.Equal(t, Verdict{Available: false, Reason: "no sandbox on native Windows; run Kstack in WSL2"}, v)

	var none *Sandbox
	assert.False(t, none.Confines())
	cmd := none.Command(context.Background(), Run{Shell: `C:\Windows\System32\cmd.exe`, Args: []string{"/c", "exit"}})
	require.Error(t, cmd.Start())
	assert.Nil(t, cmd.Process)
	_, err := none.Port()
	assert.ErrorIs(t, err, errNone)
}
