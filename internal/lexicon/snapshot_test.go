package lexicon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCached(t *testing.T) {
	dir, cache := t.TempDir(), t.TempDir()
	file := filepath.Join(dir, "a.yaml")
	write := func(s string) {
		if err := os.WriteFile(file, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("- id: sun\n  gloss: sun\n  colour: yellow\n")
	load := func() *Lexicon {
		t.Helper()
		lex, err := LoadCached(dir, cache)
		if err != nil {
			t.Fatal(err)
		}
		return lex
	}

	first := load()
	snaps, _ := filepath.Glob(filepath.Join(cache, "lexicon-*.gob"))
	if len(snaps) != 1 {
		t.Fatalf("snapshots: %v", snaps)
	}
	second := load() // from the snapshot
	if len(second.Entries) != 1 || second.Entries[0].ID != "sun" || second.Entries[0].File != file || second.Entries[0].Line != 1 {
		t.Errorf("from snapshot: %+v", second.Entries[0])
	}
	if len(second.Problems) != len(first.Problems) || len(first.Problems) != 1 {
		t.Errorf("problems: %v, then %v", first.Problems, second.Problems)
	}

	// An edit is seen at once.
	write("- id: moon\n  gloss: moon\n")
	if lex := load(); lex.Entries[0].ID != "moon" {
		t.Errorf("after edit: %s", lex.Entries[0].ID)
	}

	// A broken snapshot is ignored.
	os.WriteFile(snaps[0], []byte("junk"), 0o644)
	if lex := load(); lex.Entries[0].ID != "moon" {
		t.Errorf("after junk snapshot: %s", lex.Entries[0].ID)
	}
}
