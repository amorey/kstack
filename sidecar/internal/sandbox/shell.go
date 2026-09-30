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

// kstack-sidecar sandbox-shell: the process between a run's forwarder and its
// shell, which confines the shell and everything under it before exec'ing it.
// The filter is seccomp_linux.go; no other platform has one to install.
package sandbox

import "errors"

// ShellCommand is the subcommand that makes this executable a run's shell
// launcher.
const ShellCommand = "sandbox-shell"

// parseShellArgs reads -- <argv…>.
func parseShellArgs(args []string) ([]string, error) {
	if len(args) < 2 || args[0] != "--" {
		return nil, errors.New("no command after --")
	}
	return args[1:], nil
}
