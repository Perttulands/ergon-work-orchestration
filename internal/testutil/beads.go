package testutil

import "path/filepath"

// TestBeadsDir provisions an isolated BEADS_DIR for a test.
func TestBeadsDir(t TB) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), ".beads")
	t.Setenv("BEADS_DIR", dir)
	return dir
}
