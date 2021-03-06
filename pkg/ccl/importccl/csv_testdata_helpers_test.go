// Copyright 2019 The Cockroach Authors.
//
// Licensed as a CockroachDB Enterprise file under the Cockroach Community
// License (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
//     https://github.com/cockroachdb/cockroach/blob/master/licenses/CCL.txt

package importccl

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/util"
	"github.com/cockroachdb/cockroach/pkg/util/envutil"
)

var rewriteCSVTestData = envutil.EnvOrDefaultBool("COCKROACH_REWRITE_CSV_TESTDATA", false)

type csvTestFiles struct {
	files, gzipFiles, bzipFiles, filesWithOpts, filesWithDups []string
}

func getTestFiles(numFiles int) csvTestFiles {
	var testFiles csvTestFiles
	suffix := ""
	if util.RaceEnabled {
		suffix = "-race"
	}
	for i := 0; i < numFiles; i++ {
		testFiles.files = append(testFiles.files, fmt.Sprintf(`'nodelocal:///%s'`, fmt.Sprintf("data-%d%s", i, suffix)))
		testFiles.gzipFiles = append(testFiles.gzipFiles, fmt.Sprintf(`'nodelocal:///%s'`, fmt.Sprintf("data-%d%s.gz", i, suffix)+"?param=value"))
		testFiles.bzipFiles = append(testFiles.bzipFiles, fmt.Sprintf(`'nodelocal:///%s'`, fmt.Sprintf("data-%d%s.bz2", i, suffix)))
		testFiles.filesWithOpts = append(testFiles.filesWithOpts, fmt.Sprintf(`'nodelocal:///%s'`, fmt.Sprintf("data-%d-opts%s", i, suffix)))
		testFiles.filesWithDups = append(testFiles.filesWithDups, fmt.Sprintf(`'nodelocal:///%s'`, fmt.Sprintf("data-%d-dup%s", i, suffix)))
	}

	return testFiles
}

func makeFiles(t testing.TB, numFiles, rowsPerFile int, dir string, makeRaceFiles bool) {
	suffix := ""
	if makeRaceFiles {
		suffix = "-race"
	}

	for fn := 0; fn < numFiles; fn++ {
		// Create normal CSV file.
		fileName := filepath.Join(dir, fmt.Sprintf("data-%d%s", fn, suffix))
		f, err := os.Create(fileName)
		if err != nil {
			t.Fatal(err)
		}

		// Create CSV file which tests query options.
		fWithOpts, err := os.Create(filepath.Join(dir, fmt.Sprintf("data-%d-opts%s", fn, suffix)))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fmt.Fprint(fWithOpts, "This is a header line to be skipped\n"); err != nil {
			t.Fatal(err)
		}
		if _, err := fmt.Fprint(fWithOpts, "So is this\n"); err != nil {
			t.Fatal(err)
		}

		// Create CSV file with duplicate entries.
		fDup, err := os.Create(filepath.Join(dir, fmt.Sprintf("data-%d-dup%s", fn, suffix)))
		if err != nil {
			t.Fatal(err)
		}

		for i := 0; i < rowsPerFile; i++ {
			x := fn*rowsPerFile + i
			if _, err := fmt.Fprintf(f, "%d,%c\n", x, 'A'+x%26); err != nil {
				t.Fatal(err)
			}
			if _, err := fmt.Fprintf(fDup, "1,%c\n", 'A'+x%26); err != nil {
				t.Fatal(err)
			}

			// Write a comment.
			if _, err := fmt.Fprintf(fWithOpts, "# %d\n", x); err != nil {
				t.Fatal(err)
			}
			// Write a pipe-delim line with trailing delim.
			if x%4 == 0 { // 1/4 of rows have blank val for b
				if _, err := fmt.Fprintf(fWithOpts, "%d||\n", x); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := fmt.Fprintf(fWithOpts, "%d|%c|\n", x, 'A'+x%26); err != nil {
					t.Fatal(err)
				}
			}
		}

		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		if err := fDup.Close(); err != nil {
			t.Fatal(err)
		}
		if err := fWithOpts.Close(); err != nil {
			t.Fatal(err)
		}

		// Check in zipped versions of CSV file fileName.
		_ = gzipFile(t, fileName)
		_ = bzipFile(t, "", fileName)
	}
}

func makeCSVData(
	t testing.TB, numFiles, rowsPerFile, numRaceFiles, rowsPerRaceFile int,
) csvTestFiles {
	if rewriteCSVTestData {
		dir := filepath.Join("testdata", "csv")
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(dir, 0777); err != nil {
			t.Fatal(err)
		}

		makeFiles(t, numFiles, rowsPerFile, dir, false /* makeRaceFiles */)
		makeFiles(t, numRaceFiles, rowsPerRaceFile, dir, true)
	}

	if util.RaceEnabled {
		return getTestFiles(numRaceFiles)
	}
	return getTestFiles(numFiles)
}

func gzipFile(t testing.TB, in string) string {
	r, err := os.Open(in)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	name := in + ".gz"
	f, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := gzip.NewWriter(f)
	if _, err := io.Copy(w, r); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return name
}

func bzipFile(t testing.TB, dir, in string) string {
	_, err := exec.Command("bzip2", "-k", filepath.Join(dir, in)).CombinedOutput()
	if err != nil {
		if strings.Contains(err.Error(), "executable file not found") {
			return ""
		}
		t.Fatal(err)
	}
	return in + ".bz2"
}
