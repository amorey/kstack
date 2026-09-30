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
	"context"
	"log/slog"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/kstackhq/kstack/sidecar/internal/loginshell"
)

// importShellEnv installs the part of the user's login-shell environment we carry.
// A GUI launch inherits launchd's minimal one, so without this a kubeconfig `exec`
// credential plugin sitting on the shell PATH is not found, and a KUBECONFIG or an
// AWS_PROFILE the user exports is invisible — the same kubeconfig that works from
// a terminal fails here.
//
// Best effort: on any failure the inherited environment stands and the sidecar
// starts anyway. The log names the variables and the failure reason, never a
// value.
func importShellEnv(ctx context.Context) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, loginshell.DefaultTimeout)
	defer cancel()

	env, fault := loginshell.Import(ctx)
	if fault != nil {
		slog.Warn("shell environment not imported",
			"reason", fault.Reason,
			"exit_code", fault.ExitCode,
			"elapsed", time.Since(started),
		)
		return
	}

	names := slices.Sorted(maps.Keys(env))
	for _, name := range names {
		// Setenv fails only on a malformed name, which the compile-time list
		// rules out — so there is no half-installed environment to unwind.
		_ = os.Setenv(name, env[name])
	}
	slog.Info("shell environment imported",
		// Joined here: the log renderer reduces a value it cannot read to its
		// type, so a []string would reach the file as "<unrendered []string>".
		"variables", strings.Join(names, ","),
		"elapsed", time.Since(started),
	)
}
