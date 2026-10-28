package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const (
	mode   = "mode: atomic\n"
	module = "github.com/kstackhq/kstack/sidecar/"
)

func TestReadGoProfileCoversALineWhenAnyBlockSpanningItRan(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"p.out": mode +
		module + "internal/a.go:3.10,5.2 2 0\n" +
		module + "internal/a.go:5.2,6.3 1 1\n" +
		// No statements, so no lines.
		module + "internal/a.go:8.1,9.1 0 0\n" +
		module + "b.go:1.1,1.20 1 0\n"})

	got, err := readGoProfile(filepath.Join(dir, "p.out"))
	if err != nil {
		t.Fatal(err)
	}
	want := coverage{
		"sidecar/internal/a.go": {3: false, 4: false, 5: true, 6: true},
		"sidecar/b.go":          {1: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("readGoProfile = %v, want %v", got, want)
	}
}

func TestReadGoProfileRefusesAnEmptyOrMalformedProfile(t *testing.T) {
	for name, body := range map[string]string{
		"empty":        "",
		"no mode line": module + "a.go:1.1,2.2 1 1\n",
		"no block":     mode,
		"short line":   mode + module + "a.go:1.1,2.2 1\n",
		"bad range":    mode + module + "a.go:1,2 1 1\n",
		"bad count":    mode + module + "a.go:1.1,2.2 1 x\n",
		"other module": mode + "example.com/a.go:1.1,2.2 1 1\n",
	} {
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"p.out": body})
		path := filepath.Join(dir, "p.out")
		_, err := readGoProfile(path)
		if err == nil {
			t.Errorf("%s: readGoProfile succeeded", name)
		} else if !strings.Contains(err.Error(), path) {
			t.Errorf("%s: error %q does not name the profile", name, err)
		}
	}
}

func TestReadGoProfileNamesAMissingProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.out")
	if _, err := readGoProfile(path); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("readGoProfile(missing) = %v, want an error naming it", err)
	}
}

func TestUnionGoProfilesCoversALineAnyOSReached(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"l.out": mode + module + "a.go:1.1,2.2 2 0\n" + module + "a.go:3.1,3.9 1 1\n",
		"d.out": mode + module + "a.go:1.1,2.2 2 1\n" + module + "a.go:3.1,3.9 1 0\n" + module + "b_darwin.go:1.1,1.9 1 0\n",
		"w.out": mode + module + "a.go:1.1,2.2 2 0\n" + module + "c_windows.go:1.1,1.9 1 1\n",
	})

	got, err := unionGoProfiles(filepath.Join(dir, "l.out"), filepath.Join(dir, "d.out"), filepath.Join(dir, "w.out"))
	if err != nil {
		t.Fatal(err)
	}
	want := coverage{
		"sidecar/a.go":         {1: true, 2: true, 3: true},
		"sidecar/b_darwin.go":  {1: false},
		"sidecar/c_windows.go": {1: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unionGoProfiles = %v, want %v", got, want)
	}
}

// As scripts/coverage.go's union: some file always builds off Linux, so none
// means a profile is swapped or duplicated.
func TestUnionGoProfilesFailsWhenNothingBuildsOffLinux(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"same.out": mode + module + "a.go:1.1,2.2 2 1\n"})

	same := filepath.Join(dir, "same.out")
	if _, err := unionGoProfiles(same, same, same); err == nil {
		t.Fatal("union of one OS's profile three times succeeded")
	}
}
