package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// readLcov reads vitest's lcov. Its SF: paths are relative to the directory
// vitest ran in, the repository root, or absolute under it.
func readLcov(path, root string) (coverage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	c := coverage{}
	file := ""
	records := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "SF:"):
			file = strings.TrimPrefix(line, "SF:")
			if filepath.IsAbs(file) {
				if file, err = filepath.Rel(root, file); err != nil {
					return nil, fmt.Errorf("%s: %w", path, err)
				}
			}
			file = filepath.ToSlash(file)
			records++
		case line == "end_of_record":
			file = ""
		case strings.HasPrefix(line, "DA:"):
			// "DA:<line>,<count>[,<checksum>]"
			fields := strings.Split(strings.TrimPrefix(line, "DA:"), ",")
			if file == "" || len(fields) < 2 {
				return nil, fmt.Errorf("%s: malformed lcov line %q", path, line)
			}
			l, errL := strconv.Atoi(fields[0])
			count, errC := strconv.Atoi(fields[1])
			if errL != nil || errC != nil {
				return nil, fmt.Errorf("%s: malformed lcov line %q", path, line)
			}
			c.mark(file, l, count > 0)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if records == 0 {
		return nil, fmt.Errorf("%s: no SF: record in the lcov", path)
	}
	return c, nil
}
