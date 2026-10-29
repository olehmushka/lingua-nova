package lexicon

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"os"
	"path/filepath"
)

// LoadCached is Load through a snapshot in cacheDir: the parsed lexicon
// as gob, keyed by a hash of every lexicon file's name and bytes. Parsing
// YAML keeps line numbers and is slow (300k entries: 3 s); decoding the
// snapshot takes a twentieth of that. Any edit changes the hash, and the
// next load parses the YAML again and replaces the snapshot.
//
// A snapshot's entries keep their file and line, not the lines of their
// fields (LineOf gives the entry's line): commands that report problems in
// the files, like lnova validate, use Load. A snapshot that can't be read
// or written is ignored.
func LoadCached(dir, cacheDir string) (*Lexicon, error) {
	sum, err := filesHash(dir)
	if err != nil {
		return Load(dir) // and its error
	}
	abs, _ := filepath.Abs(dir)
	name := sha256.Sum256([]byte(abs))
	path := filepath.Join(cacheDir, "lexicon-"+hex.EncodeToString(name[:8])+".gob")

	if data, err := os.ReadFile(path); err == nil {
		var s snapshot
		if gob.NewDecoder(bytes.NewReader(data)).Decode(&s) == nil && s.Hash == sum {
			return &Lexicon{Entries: s.Entries, Problems: s.Problems}, nil
		}
	}
	lex, err := Load(dir)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if gob.NewEncoder(&buf).Encode(snapshot{sum, lex.Entries, lex.Problems}) == nil {
		writeAtomic(path, buf.Bytes())
	}
	return lex, nil
}

type snapshot struct {
	Hash     string
	Entries  []*Entry
	Problems []Problem
}

// filesHash hashes the names and bytes of the *.yaml files Load reads.
func filesHash(dir string) (string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil || len(files) == 0 {
		return "", os.ErrNotExist
	}
	h := sha256.New()
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		h.Write([]byte(f + "\x00"))
		h.Write(data)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// writeAtomic writes via a temporary file, so a reader never sees half.
func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".lexicon-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
