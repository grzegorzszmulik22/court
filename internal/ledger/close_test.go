// Copyright 2026 The Court Authors
// SPDX-License-Identifier: Apache-2.0

package ledger

import (
	"os"
	"path/filepath"
	"testing"
)

// Close must release the database file: a cached statement left open keeps
// it open, and on Windows the file can then not be removed or replaced.
func TestCloseReleasesFile(t *testing.T) {
	if err := LoadSchemas(""); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(t.TempDir(), "close.sqlite")
	s, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append("C1", "agent:a1", "tool.event", toolBody(0)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Head(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	VerifyFile(db)
	if err := os.Remove(db); err != nil {
		t.Fatalf("database file still open after Close: %v", err)
	}
}
