package logx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadTailChunkReadsIncrementally(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte("first\nsecond\n"), 0600); err != nil {
		t.Fatal(err)
	}

	first, err := ReadTailChunk(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if first.Content != "first\nsecond\n" || first.Offset != int64(len(first.Content)) || first.Reset {
		t.Fatalf("unexpected initial chunk: %#v", first)
	}

	if err := os.WriteFile(path, []byte("first\nsecond\nthird\n"), 0600); err != nil {
		t.Fatal(err)
	}
	next, err := ReadTailChunk(path, first.Offset)
	if err != nil {
		t.Fatal(err)
	}
	if next.Content != "third\n" || next.Reset {
		t.Fatalf("unexpected incremental chunk: %#v", next)
	}
}

func TestReadTailChunkResetsAfterTruncate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte("a long previous log line\n"), 0600); err != nil {
		t.Fatal(err)
	}
	chunk, err := ReadTailChunk(path, int64(len("a long previous log line\n")))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("new\n"), 0600); err != nil {
		t.Fatal(err)
	}
	reset, err := ReadTailChunk(path, chunk.Offset)
	if err != nil {
		t.Fatal(err)
	}
	if !reset.Reset || reset.Offset != int64(len("new\n")) || reset.Content != "new\n" {
		t.Fatalf("expected a reset snapshot, got %#v", reset)
	}
}

func TestReadTailChunkRejectsInvalidOffset(t *testing.T) {
	if _, err := ReadTailChunk("unused.log", -2); err == nil {
		t.Fatal("expected negative offset to be rejected")
	}
}
