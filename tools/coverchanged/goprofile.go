package main

import (
	"fmt"
	"strings"

	"golang.org/x/tools/cover"
)

// repoPath is cut off a profile's import paths, leaving the path from the
// repository root that the diff names.
const repoPath = "github.com/kstackhq/kstack/"

// readGoProfile reads a cover profile as scripts/coverage.go -out writes it.
// A line is covered when any block spanning it ran.
func readGoProfile(path string) (coverage, error) {
	profiles, err := cover.ParseProfiles(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(profiles) == 0 {
		return nil, fmt.Errorf("%s: no blocks in the profile", path)
	}
	c := coverage{}
	for _, p := range profiles {
		file, ok := strings.CutPrefix(p.FileName, repoPath)
		if !ok {
			return nil, fmt.Errorf("%s: %s is not in %s", path, p.FileName, repoPath)
		}
		for _, b := range p.Blocks {
			if b.NumStmt == 0 {
				continue
			}
			for l := b.StartLine; l <= b.EndLine; l++ {
				c.mark(file, l, b.Count > 0)
			}
		}
	}
	return c, nil
}

// unionGoProfiles reads the three OSes' profiles, as CI's coverage jobs upload
// them, into one: a line is covered when any OS reached it.
func unionGoProfiles(linux, darwin, windows string) (coverage, error) {
	u, err := readGoProfile(linux)
	if err != nil {
		return nil, err
	}
	outsideLinux := false
	for _, path := range []string{darwin, windows} {
		c, err := readGoProfile(path)
		if err != nil {
			return nil, err
		}
		for file, lines := range c {
			if u[file] == nil {
				outsideLinux = true
			}
			for l, covered := range lines {
				u.mark(file, l, covered)
			}
		}
	}
	// Some file always builds off Linux, so none means a profile is swapped or
	// duplicated, as scripts/coverage.go's union checks.
	if !outsideLinux {
		return nil, fmt.Errorf("no statements outside linux in %s and %s", darwin, windows)
	}
	return u, nil
}
