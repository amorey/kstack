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

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImportShellEnvInstallsWhatTheShellReports(t *testing.T) {
	// A stand-in login shell that exports the way a startup file would, then runs
	// the -c command for real.
	dir := t.TempDir()
	shell := filepath.Join(dir, "shell")
	script := "#!/bin/sh\nPATH='" + dir + "':\"$PATH\"\nexport PATH\n" +
		"export AWS_PROFILE=work\nexec /bin/sh -c \"$4\"\n"
	require.NoError(t, os.WriteFile(shell, []byte(script), 0o700))
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("AWS_PROFILE", "")

	importShellEnv(t.Context())

	require.Contains(t, os.Getenv("PATH"), dir)
	require.Equal(t, "work", os.Getenv("AWS_PROFILE"))
}

func TestImportShellEnvKeepsTheInheritedEnvironmentWhenResolutionFails(t *testing.T) {
	// A stand-in login shell that exits without answering.
	shell := filepath.Join(t.TempDir(), "shell")
	require.NoError(t, os.WriteFile(shell, []byte("#!/bin/sh\nexit 1\n"), 0o700))
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("AWS_PROFILE", "inherited")

	importShellEnv(t.Context())

	require.Equal(t, "/usr/bin:/bin", os.Getenv("PATH"))
	require.Equal(t, "inherited", os.Getenv("AWS_PROFILE"), "a fallback must install nothing")
}
