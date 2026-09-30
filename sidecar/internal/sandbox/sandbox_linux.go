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
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// probeBound is how long a probe's run may take. It starts this executable
// twice, which from an AppImage's mount on a cold cache takes a while.
const probeBound = 5 * time.Second

// forwarderPort is where a run's forwarder listens: the network namespace's
// loopback is the run's own, so any port is free.
const forwarderPort = 6443

// roots are the system's trees every run reads.
var roots = []string{"/usr", "/bin", "/sbin", "/lib", "/lib32", "/lib64", "/libx32", "/etc", "/opt", "/nix/store", "/home/linuxbrew/.linuxbrew"}

// systemBwraps are where a distribution puts bwrap, in the order looked for.
// bwrap is never found off PATH.
var systemBwraps = []string{"/usr/bin/bwrap", "/bin/bwrap", "/usr/local/bin/bwrap", "/run/current-system/sw/bin/bwrap"}

// Probe answers this machine's sandbox: the first bwrap that runs a command
// through the whole chain, the system's before Kstack's own.
func Probe(ctx context.Context) (*Sandbox, Verdict) {
	self, err := os.Executable()
	if err != nil {
		return nil, Verdict{Reason: "cannot find its own executable"}
	}
	return probe(ctx, self, bwrapPaths(filepath.Dir(self), systemBwraps), probeBound)
}

// probe tries each of bwraps in order and answers the first that passes, or
// every one's reason.
func probe(ctx context.Context, self string, bwraps []string, bound time.Duration) (*Sandbox, Verdict) {
	if len(bwraps) == 0 {
		return nil, Verdict{Reason: "bwrap not found"}
	}
	var reasons []string
	for _, bwrap := range bwraps {
		s := &Sandbox{self: self, bwrap: bwrap}
		if err := s.try(ctx, bound); err != nil {
			reasons = append(reasons, bwrap+": "+err.Error())
			continue
		}
		return s, Verdict{Available: true, Reason: "bwrap at " + bwrap}
	}
	return nil, Verdict{Reason: strings.Join(reasons, "; ")}
}

// try runs /bin/sh -c true through s, as a run with no cluster: bwrap, the
// forwarder, and the shell launcher's filter. Its error is the first line the
// run wrote, which names the cause.
func (s *Sandbox) try(ctx context.Context, bound time.Duration) error {
	dir, err := os.MkdirTemp("", "kstack-probe-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	home, _ := os.UserHomeDir()
	ctx, cancel := context.WithTimeout(ctx, bound)
	defer cancel()
	cmd := s.Command(ctx, Run{
		Shell: "/bin/sh", Args: []string{"-c", "true"}, Dir: dir,
		Env:       []string{"PATH=/usr/bin:/bin", "HOME=" + dir, "TMPDIR=" + dir},
		Workspace: dir, Home: home,
	})
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return fmt.Errorf("no answer in %s", bound)
	case err == nil:
		return nil
	}
	if line, _, _ := strings.Cut(stderr.String(), "\n"); line != "" {
		return errors.New(line)
	}
	return err
}

// bwrapPaths is the bwraps the probe tries, in order: the first of system
// that exists, then Kstack's own, beside the executable in dir, where it
// exists. The system's comes first because the distribution patches it, while
// Kstack's own changes only when the user installs a new release.
func bwrapPaths(dir string, system []string) []string {
	var paths []string
	for _, p := range system {
		if exists(p) {
			paths = append(paths, p)
			break
		}
	}
	if own := filepath.Join(dir, "..", "lib", "kstack", "bwrap"); exists(own) {
		paths = append(paths, own)
	}
	return paths
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Command is the process that runs r under bwrap, not yet started, made by
// exec.CommandContext on ctx. The caller sets its output, process group and
// Cancel, and starts it; exec refuses a Cancel on a command made without ctx.
func (s *Sandbox) Command(ctx context.Context, r Run) *exec.Cmd {
	cmd := exec.CommandContext(ctx, s.bwrap, s.args(r)...)
	cmd.Env = r.Env
	return cmd
}

// args is bwrap's arguments for r, in order, since a later mount lies over an
// earlier one.
func (s *Sandbox) args(r Run) []string {
	args := []string{
		"--unshare-net", "--unshare-pid", "--unshare-ipc", "--unshare-uts", "--unshare-cgroup-try",
		"--die-with-parent", "--new-session", "--as-pid-1",
		"--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp",
	}
	rootMounts, bound := rootArgs(roots)
	args = append(args, rootMounts...)

	// An entry inside a root, as written or bound, is the root's already.
	inRoots := append(slices.Clone(roots), bound...)
	entries := append(pathOf(r.Env), filepath.Dir(r.Shell))
	trees := pathTrees(r.Home, inRoots, nil, entries)
	for _, t := range trees {
		args = append(args, "--ro-bind", t, t)
	}
	for _, l := range pathLinks(inRoots, trees, entries) {
		args = append(args, "--symlink", l.target, l.path)
	}

	// Each denial lies over whatever a root or a tree made readable, a
	// directory as an empty tmpfs and a file as /dev/null.
	var sealed []string
	denials := append(credentialPaths(r.Home), resolvedAll(r.Denied)...)
	for _, d := range overlapping(denials, slices.Concat(bound, trees)) {
		info, err := os.Stat(d)
		switch {
		case err != nil:
		case info.IsDir():
			args = append(args, "--tmpfs", d)
			sealed = append(sealed, d)
		default:
			args = append(args, "--ro-bind", "/dev/null", d)
		}
	}

	for _, p := range append(slices.Clone(r.Readable), s.self) {
		args = append(args, "--ro-bind", p, p)
	}
	for _, p := range append([]string{r.Workspace}, r.Writable...) {
		args = append(args, "--bind", p, p)
	}

	// A remount is not recursive, so the binds made inside a denial stay
	// writable; it comes after them, since bwrap makes their mount points in
	// the tmpfs.
	args = append(args, "--remount-ro", "/")
	for _, d := range sealed {
		args = append(args, "--remount-ro", d)
	}
	args = append(args, "--chdir", r.Dir, "--", s.self)
	args = append(args, ForwarderArgs(r)...)
	args = append(args, s.self, ShellCommand, "--", r.Shell)
	return append(args, r.Args...)
}

// rootArgs binds each root that exists read-only at its resolved path, and
// answers the arguments and the directories bound. A root that is a link, or
// lies under one, is recreated as a link at its own path: a merged /usr makes
// /bin a link, and Fedora Atomic links /home. A target already inside a bound
// directory is not bound again.
func rootArgs(roots []string) (args, bound []string) {
	for _, root := range roots {
		at, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		if !inAny(at, bound) {
			args = append(args, "--ro-bind", at, at)
			bound = append(bound, at)
		}
		if at != root {
			args = append(args, "--symlink", at, root)
		}
	}
	return args, bound
}

// Confines reports whether a command run through s is confined: always, here.
func (s *Sandbox) Confines() bool { return true }

// Port is where a run's forwarder listens, on the namespace's own loopback.
func (s *Sandbox) Port() (int, error) { return forwarderPort, nil }
