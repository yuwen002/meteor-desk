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

func TestReadTailChunkPreservesDataAcrossMultipleChunks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	payload := bytes.Repeat([]byte("0123456789abcdef"), maxTailChunkBytes/16+37)
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}

	var got []byte
	var offset int64
	for offset < int64(len(payload)) {
		chunk, err := ReadTailChunk(path, offset)
		if err != nil {
			t.Fatal(err)
		}
		if len(chunk.Content) == 0 {
			t.Fatalf("read stopped at offset %d of %d", offset, len(payload))
		}
		if len(chunk.Content) > maxTailChunkBytes {
			t.Fatalf("chunk exceeded limit: %d", len(chunk.Content))
		}
		if chunk.Offset != offset+int64(len(chunk.Content)) {
			t.Fatalf("offset did not advance with chunk: before=%d chunk=%d after=%d", offset, len(chunk.Content), chunk.Offset)
		}
		got = append(got, chunk.Content...)
		offset = chunk.Offset
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("multi-chunk tail changed data: got %d bytes, want %d", len(got), len(payload))
	}
}

func TestReadTailChunkRejectsInvalidOffset(t *testing.T) {
	if _, err := ReadTailChunk("unused.log", -2); err == nil {
		t.Fatal("expected negative offset to be rejected")
	}
}

func TestReadTailChunkCanStartAtEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	content := []byte("history\n")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	chunk, err := ReadTailChunk(path, -1)
	if err != nil {
		t.Fatal(err)
	}
	if chunk.Content != "" || chunk.Offset != int64(len(content)) {
		t.Fatalf("expected to start at EOF, got %#v", chunk)
	}
	if err := os.WriteFile(path, append(content, []byte("new line\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	next, err := ReadTailChunk(path, chunk.Offset)
	if err != nil {
		t.Fatal(err)
	}
	if next.Content != "new line\n" {
		t.Fatalf("expected only appended content, got %#v", next)
	}
}
func TestReadTailChunkDetectsReplacedFileThatIsLarger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	first, previous, err := ReadTailChunkWithPrevious(path, -1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Offset != 3 {
		t.Fatalf("expected initial offset 3, got %#v", first)
	}

	// Keep the rotated inode alive so the filesystem cannot immediately reuse its identity.
	rotatedPath := path + ".1"
	if err := os.Rename(path, rotatedPath); err != nil {
		t.Fatal(err)
	}
	replacement := []byte("replacement file is much larger than old")
	if err := os.WriteFile(path, replacement, 0600); err != nil {
		t.Fatal(err)
	}
	chunk, _, err := ReadTailChunkWithPrevious(path, first.Offset, previous)
	if err != nil {
		t.Fatal(err)
	}
	if !chunk.Reset || chunk.Content != string(replacement) {
		t.Fatalf("expected replacement contents from offset zero, got %#v", chunk)
	}
}
