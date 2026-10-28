package main

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// newRepo makes a repository whose main branch holds files, with a branch
// feature checked out on it.
func newRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	writeFiles(t, dir, files)
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "base")
	git(t, dir, "checkout", "-q", "-b", "feature")
	return dir
}

// git runs git in dir under a fixed identity, apart from the user's config.
func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// changeEveryWay commits, stages, leaves unstaged and leaves untracked a
// change each, deletes a file, renames one, and touches files coverage does
// not count.
func changeEveryWay(t *testing.T, dir string) {
	t.Helper()
	writeFiles(t, dir, map[string]string{
		"sidecar/keep.go": "l1\nl2\nX\nl3\nl4\nl5\n",
		"README.md":       "y\n",
	})
	git(t, dir, "rm", "-q", "sidecar/del.go")
	git(t, dir, "mv", "sidecar/old.go", "sidecar/new.go")
	git(t, dir, "commit", "-q", "-am", "feature")
	writeFiles(t, dir, map[string]string{"sidecar/staged.go": "s\n"})
	git(t, dir, "add", "sidecar/staged.go")
	writeFiles(t, dir, map[string]string{
		"sidecar/keep.go":    "l1\nl2\nX\nl3\nl4\nY\n",
		"sidecar/u_test.go":  "t\n",
		"src/u.ts":           "a\nb",
		"src/u.test.ts":      "t\n",
		"tools/coverchanged": "t\n",
	})
}

var everyWayFiles = map[string]string{
	"README.md":       "x\n",
	"sidecar/keep.go": "l1\nl2\nl3\nl4\nl5\n",
	"sidecar/del.go":  "d\n",
	"sidecar/old.go":  "o1\no2\n",
}

var everyWayChanged = map[string][]int{
	"sidecar/keep.go":   {3, 6},
	"sidecar/new.go":    {1, 2},
	"sidecar/staged.go": {1},
	"src/u.ts":          {1, 2},
}

func TestParseDiffTakesTheAddedLinesOfEachHunk(t *testing.T) {
	diff := `diff --git a/sidecar/a.go b/sidecar/a.go
index 1111111..2222222 100644
--- a/sidecar/a.go
+++ b/sidecar/a.go
@@ -3,0 +4,2 @@ func a() {
+	x()
+	y()
@@ -10 +12 @@ func b() {
-	old()
+	new()
@@ -20,2 +23,0 @@ func c() {
-	gone()
-	gone()
diff --git a/src/b.ts b/src/b.ts
deleted file mode 100644
index 3333333..0000000
--- a/src/b.ts
+++ /dev/null
@@ -1,2 +0,0 @@
-a
-b
diff --git a/src/c d.ts b/src/c d.ts
new file mode 100644
index 0000000..4444444
--- /dev/null
+++ b/src/c d.ts	
@@ -0,0 +1,3 @@
+a
+b
+c
`
	got, err := parseDiff(diff)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]int{
		"sidecar/a.go": {4, 5, 12},
		"src/c d.ts":   {1, 2, 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseDiff = %v, want %v", got, want)
	}
}

func TestParseDiffRefusesAHunkOutsideAFile(t *testing.T) {
	if _, err := parseDiff("@@ -1 +1 @@\n+x\n"); err == nil {
		t.Fatal("parseDiff of a hunk with no +++ line succeeded")
	}
}

func TestLanguageOfCountsOnlyNonTestSourceUnderSidecarAndSrc(t *testing.T) {
	for path, want := range map[string]language{
		"sidecar/internal/app/app.go":      goLang,
		"sidecar/internal/app/app_test.go": "",
		"sidecar/go.mod":                   "",
		"src/lib/a.ts":                     tsLang,
		"src/components/b.tsx":             tsLang,
		"src/lib/a.test.ts":                "",
		"src/components/b.test.tsx":        "",
		"src/styles.css":                   "",
		"scripts/coverage-js.go":           "",
		"tools/coverchanged/main.go":       "",
		"vite.config.ts":                   "",
	} {
		if got := languageOf(path); got != want {
			t.Errorf("languageOf(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestChangedLinesTakesEveryKindOfChangeSinceTheMergeBase(t *testing.T) {
	dir := newRepo(t, everyWayFiles)
	changeEveryWay(t, dir)

	got, err := changedLines(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, everyWayChanged) {
		t.Fatalf("changedLines = %v, want %v", got, everyWayChanged)
	}
}

func TestChangedLinesIgnoresTheDevelopersGitConfig(t *testing.T) {
	dir := newRepo(t, everyWayFiles)
	for _, kv := range [][2]string{
		{"color.ui", "always"},
		{"color.diff", "always"},
		{"diff.noprefix", "true"},
		{"diff.mnemonicPrefix", "true"},
		{"diff.relative", "true"},
		{"diff.renames", "copies"},
		{"diff.external", "false"},
		{"core.quotePath", "true"},
	} {
		git(t, dir, "config", kv[0], kv[1])
	}
	changeEveryWay(t, dir)
	writeFiles(t, dir, map[string]string{"src/ñandú.ts": "a\n"})
	git(t, dir, "add", "src/ñandú.ts")

	got, err := changedLines(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]int{"src/ñandú.ts": {1}}
	maps.Copy(want, everyWayChanged)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("changedLines = %v, want %v", got, want)
	}
}

func TestResolveBaseTakesTheFlagThenBASEThenUpstreamThenOrigin(t *testing.T) {
	dir := newRepo(t, map[string]string{"a": "a\n"})

	if got := resolveBase(dir, "", ""); got != "origin/main" {
		t.Errorf("with no upstream remote, base = %q, want origin/main", got)
	}
	git(t, dir, "update-ref", "refs/remotes/upstream/main", "HEAD")
	if got := resolveBase(dir, "", ""); got != "upstream/main" {
		t.Errorf("with an upstream remote, base = %q, want upstream/main", got)
	}
	if got := resolveBase(dir, "", "env"); got != "env" {
		t.Errorf("with BASE set, base = %q, want env", got)
	}
	if got := resolveBase(dir, "flag", "env"); got != "flag" {
		t.Errorf("with -base given, base = %q, want flag", got)
	}
}
