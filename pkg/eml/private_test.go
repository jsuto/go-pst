package eml

import (
	"os"
	"path/filepath"
	"testing"
)

// PrivateTestPST returns the path of the private test PST: backup.pst in the directory GO_PST_TESTDATA.
// The file has real addresses and is kept outside of the repository; the test is skipped without it.
// Exported for the tests of package eml_test.
func PrivateTestPST(tb testing.TB) string {
	directory := os.Getenv("GO_PST_TESTDATA")

	if directory == "" {
		tb.Skip("GO_PST_TESTDATA is not set (directory of the private test PST)")
	}

	return filepath.Join(directory, "backup.pst")
}
