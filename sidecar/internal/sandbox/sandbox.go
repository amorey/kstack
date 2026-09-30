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

// Package sandbox runs a command in this machine's sandbox. It knows no tool and
// no cluster. Each platform's file supplies Probe, Command, Confines and Port,
// and InitMain, a run's forwarder (forward.go).
package sandbox

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
)

// Main runs this executable as the part of a run its first argument names,
// sandbox-init or sandbox-shell, and answers the exit code. ok is false for
// any other command line, which is the caller's own. Every binary a run can
// start calls it first: the sidecar, and each test binary whose tests do.
func Main(argv []string) (code int, ok bool) {
	if len(argv) < 2 {
		return 0, false
	}
	switch argv[1] {
	case InitCommand:
		return InitMain(argv[2:]), true
	case ShellCommand:
		return ShellMain(argv[2:]), true
	}
	return 0, false
}

// Run is one command to start sandboxed.
type Run struct {
	Shell string   // the shell's path
	Args  []string // what the shell is given: -c and the wrapper
	Dir   string   // where it starts
	Env   []string // the whole environment

	// What a platform mounts.
	Workspace string   // writable, and HOME
	Readable  []string // beyond the system roots and PATH
	Writable  []string // beyond the workspace
	Home      string   // the user's home, unreadable but for the PATH trees in it
	Denied    []string // Kstack's own directories, unreadable but for Readable and Writable
	Port      int      // where the run's kubeconfig dials its forwarder

	// Socket is the run's proxy socket, which the forwarder relays Port to;
	// empty for a run with no cluster, whose forwarder relays nothing.
	Socket string
}

// Check is why r cannot run, or nil: one of its own paths is a link. A
// profile resolves each path, so a link a run planted where its workspace was
// would open the link's target to the next. Only the last component is
// checked: a link above it, such as macOS's /var, is the system's. A path
// that is not there opens nothing and is passed over.
func (r Run) Check() error {
	for _, p := range slices.Concat([]string{r.Workspace}, r.Writable, r.Readable) {
		info, err := os.Lstat(p)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return err
		case info.Mode()&fs.ModeSymlink != 0:
			return fmt.Errorf("%s is a link", p)
		}
	}
	return nil
}

// Verdict is whether this machine has a sandbox for commands to run through,
// and why not. Whether that sandbox confines them is Sandbox.Confines.
type Verdict struct {
	Available bool
	Reason    string
}

// Sandbox is this machine's sandbox. A nil *Sandbox is none.
type Sandbox struct {
	// self is this executable, which runs as a run's forwarder.
	self string
	// bwrap is the bwrap a run starts under, on Linux.
	bwrap string
	// launcher is the sandbox-exec a run starts under, on macOS.
	launcher string
}
