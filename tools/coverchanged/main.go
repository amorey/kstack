// Command coverchanged lists the lines a branch changed that no test reaches,
// as file:lines, and exits 1 when there are any. make cover-changed runs it,
// measuring this machine; CI's Coverage · Changed lines runs it with -no-run on
// the coverage jobs' uploads.
package main

import (
	"errors"
	"flag"
	"fmt"
	"go/build"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, measureSuite))
}

// measureFunc runs lang's suite under coverage in the repository at root,
// writes what it measured to out, and sends the suite's output to w.
type measureFunc func(root string, lang language, out string, w io.Writer) error

// errUsage is a usage error; parseFlags has printed it already.
var errUsage = errors.New("usage")

// run is the whole command. It returns the exit code: 1 when a changed line is
// not covered or the run fails, 2 on a usage error.
func run(args []string, stdout, stderr io.Writer, measure measureFunc) int {
	err := check(args, stdout, stderr, measure)
	switch {
	case errors.Is(err, errUsage):
		return 2
	case err != nil:
		fmt.Fprintln(stderr, "cover-changed:", err)
		return 1
	}
	return 0
}

// options are the command's flags.
type options struct {
	base                   string
	noRun                  bool
	linux, darwin, windows string
	lcov                   string
}

func parseFlags(args []string, stderr io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet("cover-changed", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.base, "base", "", "diff against `ref`; default BASE, else upstream/main when it exists, else origin/main")
	fs.BoolVar(&o.noRun, "no-run", false, "read the inputs given and run no tests")
	fs.StringVar(&o.linux, "linux", "", "Linux cover profile, with -darwin and -windows (-no-run)")
	fs.StringVar(&o.darwin, "darwin", "", "macOS cover profile (-no-run)")
	fs.StringVar(&o.windows, "windows", "", "Windows cover profile (-no-run)")
	fs.StringVar(&o.lcov, "lcov", "", "vitest's lcov (-no-run)")
	if err := fs.Parse(args); err != nil {
		return options{}, errUsage
	}

	anyProfile := o.linux != "" || o.darwin != "" || o.windows != ""
	allProfiles := o.linux != "" && o.darwin != "" && o.windows != ""
	var msg string
	switch {
	case fs.NArg() > 0:
		msg = "takes no arguments"
	case !o.noRun && (anyProfile || o.lcov != ""):
		msg = "-linux, -darwin, -windows and -lcov need -no-run"
	case anyProfile && !allProfiles:
		msg = "-linux, -darwin and -windows go together"
	}
	if msg != "" {
		fmt.Fprintln(stderr, "cover-changed:", msg)
		fs.Usage()
		return options{}, errUsage
	}
	return o, nil
}

func check(args []string, stdout, stderr io.Writer, measure measureFunc) error {
	o, err := parseFlags(args, stderr)
	if err != nil {
		return err
	}
	root, err := gitOutput(".", "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	root = strings.TrimSpace(root)
	changed, err := changedLines(root, resolveBase(root, o.base, os.Getenv("BASE")))
	if err != nil {
		return err
	}

	var c coverage
	if o.noRun {
		c, err = readInputs(o, root, changed)
	} else {
		c, err = measureHere(root, changed, stderr, measure)
	}
	if err != nil {
		return err
	}
	if n := report(stdout, changed, c); n > 0 {
		return fmt.Errorf("%d changed %s not covered; add tests that check their behaviour", n, plural(n))
	}
	return nil
}

// readInputs reads the coverage given with -no-run. Every changed file's
// language must have its input, or the file would pass unchecked.
func readInputs(o options, root string, changed map[string][]int) (coverage, error) {
	given := map[language]bool{goLang: o.linux != "", tsLang: o.lcov != ""}
	for _, file := range slices.Sorted(maps.Keys(changed)) {
		if lang := languageOf(file); !given[lang] {
			return nil, fmt.Errorf("%s changed, and no %s coverage was given", file, lang)
		}
	}
	c := coverage{}
	if given[goLang] {
		goCov, err := unionGoProfiles(o.linux, o.darwin, o.windows)
		if err != nil {
			return nil, err
		}
		maps.Copy(c, goCov)
	}
	if given[tsLang] {
		tsCov, err := readLcov(o.lcov, root)
		if err != nil {
			return nil, err
		}
		maps.Copy(c, tsCov)
	}
	return c, nil
}

// measureHere measures the suite of each language with a changed file on this
// machine, then notes the changed Go files only CI measures.
func measureHere(root string, changed map[string][]int, w io.Writer, measure measureFunc) (coverage, error) {
	tmp, err := os.MkdirTemp("", "cover-changed-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	langs := map[language]bool{}
	for file := range changed {
		langs[languageOf(file)] = true
	}
	c := coverage{}
	for _, lang := range []language{goLang, tsLang} {
		if !langs[lang] {
			continue
		}
		out := filepath.Join(tmp, string(lang))
		if err := measure(root, lang, out, w); err != nil {
			return nil, fmt.Errorf("measuring %s: %w", lang, err)
		}
		var measured coverage
		if lang == goLang {
			measured, err = readGoProfile(out)
		} else {
			measured, err = readLcov(out, root)
		}
		if err != nil {
			return nil, err
		}
		maps.Copy(c, measured)
	}
	if err := noteNotMeasured(w, root, changed); err != nil {
		return nil, err
	}
	return c, nil
}

// measureSuite runs scripts/coverage.go or scripts/coverage-js.go, so a local
// run starts each suite as CI's coverage jobs do.
func measureSuite(root string, lang language, out string, w io.Writer) error {
	var cmd *exec.Cmd
	if lang == goLang {
		cmd = exec.Command("go", "run", "scripts/coverage.go", "-out", out)
		cmd.Dir = filepath.Join(root, "sidecar")
	} else {
		cmd = exec.Command("go", "run", "scripts/coverage-js.go", "-nogate", "-lcov", out)
		cmd.Dir = root
	}
	cmd.Stdout, cmd.Stderr = w, w
	return cmd.Run()
}

// noteNotMeasured says which changed Go files this machine does not build but
// a CI target does. They have no blocks here, so they pass here.
func noteNotMeasured(w io.Writer, root string, changed map[string][]int) error {
	local := build.Default
	local.GOOS, local.GOARCH = runtime.GOOS, runtime.GOARCH
	for _, file := range slices.Sorted(maps.Keys(changed)) {
		if languageOf(file) != goLang {
			continue
		}
		only, err := onlyCIMeasures(filepath.Join(root, file), local)
		if err != nil {
			return err
		}
		if only {
			fmt.Fprintf(w, "cover-changed: %s not measured on %s; CI checks it\n", file, runtime.GOOS)
		}
	}
	return nil
}

// ciTargets are the go-coverage job's runners in .github/workflows/ci.yml;
// keep them in step. CI measures with CGO_ENABLED=0.
var ciTargets = []struct{ goos, goarch string }{
	{"linux", "amd64"},
	{"darwin", "arm64"},
	{"windows", "amd64"},
}

// onlyCIMeasures reports whether local builds the Go file at path in neither
// the untagged nor the debug build while some CI target does. A file no CI
// target builds is not checked anywhere.
func onlyCIMeasures(path string, local build.Context) (bool, error) {
	here, err := builds(local, path)
	if err != nil || here {
		return false, err
	}
	for _, target := range ciTargets {
		ctx := build.Default
		ctx.GOOS, ctx.GOARCH, ctx.CgoEnabled = target.goos, target.goarch, false
		if ok, err := builds(ctx, path); err != nil || ok {
			return ok, err
		}
	}
	return false, nil
}

// builds reports whether ctx builds path untagged or with -tags debug, the two
// builds scripts/coverage.go measures.
func builds(ctx build.Context, path string) (bool, error) {
	for _, tags := range [][]string{nil, {"debug"}} {
		ctx.BuildTags = tags
		ok, err := ctx.MatchFile(filepath.Dir(path), filepath.Base(path))
		if err != nil || ok {
			return ok, err
		}
	}
	return false, nil
}

// coverage maps each measured file, relative to the repository root, to its
// statement lines and whether a test reached each. A line it lacks is not a
// statement.
type coverage map[string]map[int]bool

// mark records one measurement of a line; any run that reached it covers it.
func (c coverage) mark(file string, line int, covered bool) {
	if c[file] == nil {
		c[file] = map[int]bool{}
	}
	c[file][line] = c[file][line] || covered
}

// report prints one line per run of consecutive changed lines no test reached,
// by path then line, and returns how many lines it listed. A file or line the
// coverage lacks has no statement to reach.
func report(w io.Writer, changed map[string][]int, c coverage) int {
	tw := tabwriter.NewWriter(w, 0, 0, 4, ' ', 0)
	n := 0
	for _, file := range slices.Sorted(maps.Keys(changed)) {
		var missed []int
		for _, l := range changed[file] {
			if covered, measured := c[file][l]; measured && !covered {
				missed = append(missed, l)
			}
		}
		slices.Sort(missed)
		n += len(missed)
		for _, r := range ranges(missed) {
			fmt.Fprintf(tw, "%s:%s\tnot covered\n", file, r)
		}
	}
	tw.Flush()
	return n
}

// ranges writes sorted lines as runs: 3, 4, 5 and 8 become "3-5" and "8".
func ranges(lines []int) []string {
	var out []string
	for i := 0; i < len(lines); {
		j := i
		for j+1 < len(lines) && lines[j+1] == lines[j]+1 {
			j++
		}
		if i == j {
			out = append(out, strconv.Itoa(lines[i]))
		} else {
			out = append(out, fmt.Sprintf("%d-%d", lines[i], lines[j]))
		}
		i = j + 1
	}
	return out
}

func plural(n int) string {
	if n == 1 {
		return "line is"
	}
	return "lines are"
}
