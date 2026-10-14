package logx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchFileLiteralAndLineNumbers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.log")
	if err := os.WriteFile(path, []byte("booting\nINFO signed in\nWARN timeout\nINFO ready"), 0600); err != nil { t.Fatal(err) }
	got, err := SearchFile(path, "INFO", false, 10)
	if err != nil { t.Fatal(err) }
	if got.ScannedLines != 4 { t.Fatalf("ScannedLines = %d, want 4", got.ScannedLines) }
	if len(got.Matches) != 2 { t.Fatalf("matches = %d, want 2", len(got.Matches)) }
	if got.Matches[0].LineNumber != 2 || got.Matches[1].LineNumber != 4 { t.Fatalf("unexpected matches: %+v", got.Matches) }
}

func TestSearchFileRegexAndResultLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.log")
	if err := os.WriteFile(path, []byte("error 100\nerror 200\ninfo 300\nerror 400\n"), 0600); err != nil { t.Fatal(err) }
	got, err := SearchFile(path, "^error [0-9]+$", true, 2)
	if err != nil { t.Fatal(err) }
	if len(got.Matches) != 2 || !got.ResultLimitReached { t.Fatalf("unexpected result: %+v", got) }
}

func TestSearchFileClipsVeryLongLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "long.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", maxPreviewBytes+100)), 0600); err != nil { t.Fatal(err) }
	got, err := SearchFile(path, "xxx", false, 1)
	if err != nil { t.Fatal(err) }
	if len(got.Matches) != 1 || !got.Matches[0].ContentClipped { t.Fatalf("expected clipped match, got %+v", got) }
}
