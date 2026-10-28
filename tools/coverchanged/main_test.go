package main

import (
	"errors"
	"fmt"
	"go/build"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const goFile = "package a\n\nfunc A() int {\n\treturn 1\n}\n"

// branchRepo makes a repository whose feature branch, checked out and the
// working directory, adds sidecar/a.go and the files given.
func branchRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := newRepo(t, map[string]string{"README.md": "x\n"})
	writeFiles(t, dir, map[string]string{"sidecar/a.go": goFile})
	writeFiles(t, dir, files)
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "feature")
	t.Chdir(dir)
	return dir
}

// noMeasure fails a test whose run should start no suite.
func noMeasure(t *testing.T) measureFunc {
	return func(string, language, string, io.Writer) error {
		t.Error("measured")
		return nil
	}
}

// profiles writes a Go profile per OS: a.go's lines 3-5 run count times, and
// one file off Linux so the union holds.
func profiles(t *testing.T, count string) []string {
	t.Helper()
	dir := t.TempDir()
	block := mode + module + "a.go:3.14,5.2 1 " + count + "\n"
	writeFiles(t, dir, map[string]string{
		"l.out": block,
		"d.out": block + module + "b_darwin.go:1.1,1.9 1 1\n",
		"w.out": block,
	})
	return []string{
		"-linux", filepath.Join(dir, "l.out"),
		"-darwin", filepath.Join(dir, "d.out"),
		"-windows", filepath.Join(dir, "w.out"),
	}
}

type result struct {
	code           int
	stdout, stderr string
}

func runWith(args []string, measure measureFunc) result {
	var stdout, stderr strings.Builder
	code := run(args, &stdout, &stderr, measure)
	return result{code, stdout.String(), stderr.String()}
}

func TestReportListsEachRunOfUncoveredChangedLinesInOrder(t *testing.T) {
	changed := map[string][]int{
		"src/b.ts":      {1},
		"sidecar/a.go":  {1, 2, 3, 4, 5, 6, 9},
		"sidecar/c.go":  {1}, // not measured
		"sidecar/ok.go": {1},
	}
	c := coverage{
		// Line 4 is no statement; line 7 is not changed.
		"sidecar/a.go":  {1: false, 2: false, 3: false, 5: true, 6: false, 7: false, 9: false},
		"sidecar/ok.go": {1: true},
		"src/b.ts":      {1: false},
	}

	var out strings.Builder
	n := report(&out, changed, c)
	want := `sidecar/a.go:1-3    not covered
sidecar/a.go:6      not covered
sidecar/a.go:9      not covered
src/b.ts:1          not covered
`
	if out.String() != want {
		t.Errorf("report printed\n%s\nwant\n%s", out.String(), want)
	}
	if n != 6 {
		t.Errorf("report counted %d lines, want 6", n)
	}
}

func TestNoRunListsTheChangedLinesTheProfilesMissed(t *testing.T) {
	branchRepo(t, nil)

	got := runWith(append([]string{"-no-run", "-base", "main"}, profiles(t, "0")...), noMeasure(t))
	if got.code != 1 || got.stdout != "sidecar/a.go:3-5    not covered\n" {
		t.Fatalf("run = %+v, want exit 1 listing a.go:3-5", got)
	}
	if !strings.Contains(got.stderr, "3 changed lines are not covered") {
		t.Errorf("stderr = %q, want the count", got.stderr)
	}

	got = runWith(append([]string{"-no-run", "-base", "main"}, profiles(t, "1")...), noMeasure(t))
	if got.code != 0 || got.stdout != "" {
		t.Fatalf("run = %+v, want exit 0 and nothing listed", got)
	}
}

func TestNoRunFailsWithoutTheInputOfAChangedLanguage(t *testing.T) {
	branchRepo(t, map[string]string{"src/b.ts": "export const b = 1;\n"})

	for name, args := range map[string][]string{
		"no inputs":      {"-no-run", "-base", "main"},
		"no lcov":        append([]string{"-no-run", "-base", "main"}, profiles(t, "1")...),
		"no Go profiles": {"-no-run", "-base", "main", "-lcov", lcovFile(t, "SF:src/b.ts\nDA:1,1\nend_of_record\n")},
	} {
		if got := runWith(args, noMeasure(t)); got.code != 1 {
			t.Errorf("%s: run = %+v, want exit 1", name, got)
		}
	}
}

func TestNoRunWithNoChangedFilePassesWithNoInputs(t *testing.T) {
	dir := newRepo(t, map[string]string{"README.md": "x\n"})
	t.Chdir(dir)

	if got := runWith([]string{"-no-run", "-base", "main"}, noMeasure(t)); got.code != 0 {
		t.Fatalf("run = %+v, want exit 0", got)
	}
}

func TestNoRunNamesAMissingInput(t *testing.T) {
	branchRepo(t, nil)
	missing := filepath.Join(t.TempDir(), "coverage-windows.out")
	args := append([]string{"-no-run", "-base", "main"}, profiles(t, "1")...)
	args[len(args)-1] = missing

	got := runWith(args, noMeasure(t))
	if got.code != 1 || !strings.Contains(got.stderr, missing) {
		t.Fatalf("run = %+v, want exit 1 naming %s", got, missing)
	}
}

func TestUsageErrors(t *testing.T) {
	branchRepo(t, nil)
	two := profiles(t, "1")[:4]

	for name, args := range map[string][]string{
		"two of three Go profiles": append([]string{"-no-run"}, two...),
		"an input while measuring": {"-lcov", "lcov.info"},
		"an argument":              {"extra"},
	} {
		if got := runWith(args, noMeasure(t)); got.code != 2 {
			t.Errorf("%s: run = %+v, want exit 2", name, got)
		}
	}
}

func lcovFile(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"lcov.info": body})
	return filepath.Join(dir, "lcov.info")
}

// fakeMeasure writes a measurement of a.go and src/b.ts with everything
// covered, prints to the writer it is given, and records each language.
func fakeMeasure(measured *[]language) measureFunc {
	return func(root string, lang language, out string, w io.Writer) error {
		*measured = append(*measured, lang)
		fmt.Fprintf(w, "measuring %s\n", lang)
		body := mode + module + "a.go:3.14,5.2 1 1\n"
		if lang == tsLang {
			body = "SF:src/b.ts\nDA:1,1\nend_of_record\n"
		}
		return os.WriteFile(out, []byte(body), 0o644)
	}
}

func TestMeasuringACoveredChangeListsNothing(t *testing.T) {
	branchRepo(t, nil)
	var measured []language

	got := runWith([]string{"-base", "main"}, fakeMeasure(&measured))
	if got.code != 0 || got.stdout != "" {
		t.Fatalf("run = %+v, want exit 0 and nothing listed", got)
	}
	if !strings.Contains(got.stderr, "measuring Go") {
		t.Errorf("stderr = %q, want the measurement's output", got.stderr)
	}
	if !slices.Equal(measured, []language{goLang}) {
		t.Errorf("measured %v, want Go alone", measured)
	}
}

func TestMeasuringASrcOnlyChangeMeasuresTypeScriptAlone(t *testing.T) {
	dir := newRepo(t, map[string]string{"README.md": "x\n"})
	writeFiles(t, dir, map[string]string{"src/b.ts": "export const b = 1;\n"})
	t.Chdir(dir)
	var measured []language

	if got := runWith([]string{"-base", "main"}, fakeMeasure(&measured)); got.code != 0 {
		t.Fatalf("run = %+v, want exit 0", got)
	}
	if !slices.Equal(measured, []language{tsLang}) {
		t.Errorf("measured %v, want TypeScript alone", measured)
	}
}

func TestMeasuringNoCountedChangeMeasuresNothing(t *testing.T) {
	dir := newRepo(t, map[string]string{"README.md": "x\n"})
	writeFiles(t, dir, map[string]string{"README.md": "y\n", "sidecar/a_test.go": goFile})
	t.Chdir(dir)

	if got := runWith([]string{"-base", "main"}, noMeasure(t)); got.code != 0 {
		t.Fatalf("run = %+v, want exit 0", got)
	}
}

func TestAFailedMeasurementFailsTheRun(t *testing.T) {
	branchRepo(t, nil)
	failing := func(string, language, string, io.Writer) error { return errors.New("tests failed") }

	if got := runWith([]string{"-base", "main"}, failing); got.code != 1 || got.stdout != "" {
		t.Fatalf("run = %+v, want exit 1 and nothing listed", got)
	}
}

func TestOnlyCIMeasuresAFileThisMachineDoesNotBuild(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"a.go":              "package a\n",
		"a_windows.go":      "package a\n",
		"a_darwin_amd64.go": "package a\n",
		"debug.go":          "//go:build debug\n\npackage a\n",
		"cgo.go":            "//go:build cgo\n\npackage a\n",
		"ignored.go":        "//go:build ignore\n\npackage main\n",
	})
	local := build.Default
	local.GOOS, local.GOARCH, local.CgoEnabled = "linux", "amd64", false

	for file, want := range map[string]bool{
		"a.go":         false,
		"a_windows.go": true,
		// No CI target is darwin/amd64, and none builds with cgo.
		"a_darwin_amd64.go": false,
		"cgo.go":            false,
		"debug.go":          false,
		"ignored.go":        false,
	} {
		got, err := onlyCIMeasures(filepath.Join(dir, file), local)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("onlyCIMeasures(%s) = %v, want %v", file, got, want)
		}
	}
}
