package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReadLcovMapsEachFileToItsLinesFromTheRoot(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "repo")
	writeFiles(t, dir, map[string]string{"lcov.info": "TN:\n" +
		"SF:src/a.ts\nFN:1,f\nFNDA:1,f\nDA:1,1\nDA:2,0\nLF:2\nLH:1\nend_of_record\n" +
		"SF:" + filepath.Join(root, "src", "b.tsx") + "\nDA:5,0\nend_of_record\n" +
		// Recorded twice: covered when either record reached it.
		"SF:src/a.ts\nDA:2,3\nDA:3,0,abc\nend_of_record\n"})

	got, err := readLcov(filepath.Join(dir, "lcov.info"), root)
	if err != nil {
		t.Fatal(err)
	}
	want := coverage{
		"src/a.ts":  {1: true, 2: true, 3: false},
		"src/b.tsx": {5: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("readLcov = %v, want %v", got, want)
	}
}

func TestReadLcovRefusesAnEmptyOrMalformedLcov(t *testing.T) {
	for name, body := range map[string]string{
		"empty":         "",
		"no SF":         "TN:\n",
		"DA outside SF": "DA:1,1\n",
		"bad DA":        "SF:src/a.ts\nDA:x,1\nend_of_record\n",
		"short DA":      "SF:src/a.ts\nDA:1\nend_of_record\n",
	} {
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"lcov.info": body})
		path := filepath.Join(dir, "lcov.info")
		_, err := readLcov(path, dir)
		if err == nil {
			t.Errorf("%s: readLcov succeeded", name)
		} else if !strings.Contains(err.Error(), path) {
			t.Errorf("%s: error %q does not name the lcov", name, err)
		}
	}
}

func TestReadLcovNamesAMissingLcov(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lcov.info")
	if _, err := readLcov(path, t.TempDir()); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("readLcov(missing) = %v, want an error naming it", err)
	}
}
