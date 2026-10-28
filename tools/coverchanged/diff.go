package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// resolveBase picks the branch to diff against as scripts/test-changed.sh
// does: -base, else BASE, else upstream/main when it exists, since a fork's
// origin/main can lag, else origin/main.
func resolveBase(root, flagBase, envBase string) string {
	switch {
	case flagBase != "":
		return flagBase
	case envBase != "":
		return envBase
	}
	if _, err := gitOutput(root, "rev-parse", "-q", "--verify", "upstream/main"); err == nil {
		return "upstream/main"
	}
	return "origin/main"
}

// changedLines lists the lines added to the files coverage counts (languageOf)
// since root's HEAD left base: committed, staged, unstaged or untracked, by
// file relative to root.
func changedLines(root, base string) (map[string][]int, error) {
	mergeBase, err := gitOutput(root, "merge-base", base, "HEAD")
	if err != nil {
		return nil, err
	}
	// Every flag that a developer's config could change is set, so the output
	// always parses, and parses the same as in CI.
	diff, err := gitOutput(root, "-c", "core.quotePath=false", "diff", "-U0", "--no-renames", "--no-color",
		"--no-ext-diff", "--no-relative", "--diff-algorithm=myers", "--indent-heuristic",
		"--src-prefix=a/", "--dst-prefix=b/", strings.TrimSpace(mergeBase))
	if err != nil {
		return nil, err
	}
	changed, err := parseDiff(diff)
	if err != nil {
		return nil, err
	}
	for file := range changed {
		if languageOf(file) == "" {
			delete(changed, file)
		}
	}
	untracked, err := gitOutput(root, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	for _, file := range strings.Split(untracked, "\x00") {
		if languageOf(file) == "" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			return nil, err
		}
		for range bytes.Lines(raw) {
			changed[file] = append(changed[file], len(changed[file])+1)
		}
	}
	return changed, nil
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// parseDiff reads `git diff -U0` output into the added lines of each file. A
// file only deleted from has none.
func parseDiff(diff string) (map[string][]int, error) {
	changed := map[string][]int{}
	file := ""
	for line := range strings.Lines(diff) {
		line = strings.TrimSuffix(line, "\n")
		switch {
		case strings.HasPrefix(line, "diff --git "):
			file = ""
		case strings.HasPrefix(line, "+++ "):
			// Git ends a name holding a space with a tab.
			name := strings.TrimSuffix(strings.TrimPrefix(line, "+++ "), "\t")
			file = strings.TrimPrefix(name, "b/") // "/dev/null" stays itself
		case strings.HasPrefix(line, "@@ "):
			if file == "" {
				return nil, fmt.Errorf("diff: hunk with no file: %q", line)
			}
			start, n, err := addedRange(line)
			if err != nil {
				return nil, err
			}
			for l := start; l < start+n; l++ {
				changed[file] = append(changed[file], l)
			}
		}
	}
	delete(changed, "/dev/null")
	return changed, nil
}

// addedRange reads the "+start,count" of a hunk header; a missing count is 1.
func addedRange(header string) (start, n int, err error) {
	fields := strings.Fields(header)
	if len(fields) < 3 || !strings.HasPrefix(fields[2], "+") {
		return 0, 0, fmt.Errorf("diff: malformed hunk header %q", header)
	}
	from, count, found := strings.Cut(fields[2][1:], ",")
	if !found {
		count = "1"
	}
	start, errStart := strconv.Atoi(from)
	n, errCount := strconv.Atoi(count)
	if errStart != nil || errCount != nil {
		return 0, 0, fmt.Errorf("diff: malformed hunk header %q", header)
	}
	return start, n, nil
}

// language is the coverage a changed file is checked against; "" is none.
type language string

const (
	goLang language = "Go"
	tsLang language = "TypeScript"
)

// languageOf says which coverage counts a file, or none: the tests and every
// file outside the sidecar and the webview are not checked.
func languageOf(path string) language {
	switch {
	case strings.HasPrefix(path, "sidecar/") && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go"):
		return goLang
	case strings.HasPrefix(path, "src/") && (strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx")) &&
		!strings.HasSuffix(path, ".test.ts") && !strings.HasSuffix(path, ".test.tsx"):
		return tsLang
	}
	return ""
}
