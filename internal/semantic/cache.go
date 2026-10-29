package semantic

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// The cache file holds one record per embedded text, at the model's full
// dimensions, int8-quantized:
//
//	header:  "lnova-emb\x01", model name (uint16 length + bytes), dims (uint32)
//	record:  text hash [16]byte, scale float32, codes [dims]int8
//
// It is append-only while embedding: every batch is appended and synced,
// so an interrupted build of 300k entries resumes where it stopped instead
// of starting over (the idea of a write-ahead log). A later record for the
// same hash wins. When the file holds records no entry needs any more
// (edited or deleted entries), Build rewrites it with only the live ones.

const magic = "lnova-emb\x01"

// key is a text's hash: what a record is looked up by.
type key [16]byte

func hash(text string) key {
	h := sha256.Sum256([]byte(text))
	return key(h[:16])
}

type record struct {
	scale float32
	codes []int8 // full dims
}

// cacheHeader is the part of a cache file before the records.
type cacheHeader struct {
	model string
	dims  int
}

func (h cacheHeader) recordSize() int { return 16 + 4 + h.dims }

func (h cacheHeader) write(w io.Writer) error {
	var b bytes.Buffer
	b.WriteString(magic)
	binary.Write(&b, binary.LittleEndian, uint16(len(h.model)))
	b.WriteString(h.model)
	binary.Write(&b, binary.LittleEndian, uint32(h.dims))
	_, err := w.Write(b.Bytes())
	return err
}

func readHeader(r io.Reader) (cacheHeader, error) {
	var h cacheHeader
	m := make([]byte, len(magic))
	if _, err := io.ReadFull(r, m); err != nil || string(m) != magic {
		return h, errors.New("not an embedding cache")
	}
	var n uint16
	if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
		return h, err
	}
	name := make([]byte, n)
	if _, err := io.ReadFull(r, name); err != nil {
		return h, err
	}
	var d uint32
	if err := binary.Read(r, binary.LittleEndian, &d); err != nil {
		return h, err
	}
	return cacheHeader{string(name), int(d)}, nil
}

func (h cacheHeader) writeRecord(w io.Writer, k key, r record) error {
	buf := make([]byte, h.recordSize())
	copy(buf, k[:])
	binary.LittleEndian.PutUint32(buf[16:], math.Float32bits(r.scale))
	for i, c := range r.codes {
		buf[20+i] = byte(c)
	}
	_, err := w.Write(buf)
	return err
}

// readCache calls each for every complete record of the file at path, if
// it is a cache of the model; a missing, foreign or damaged file has no
// records. A cut-off last record (a build killed mid-write) is ignored.
// It returns the header and the number of records read.
func readCache(path, model string, each func(key, record)) (cacheHeader, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return cacheHeader{}, 0, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)
	h, err := readHeader(r)
	if err != nil {
		return h, 0, err
	}
	if h.model != model {
		return h, 0, fmt.Errorf("cache of model %q", h.model)
	}
	buf := make([]byte, h.recordSize())
	n := 0
	for {
		if _, err := io.ReadFull(r, buf); err != nil {
			return h, n, nil // EOF, or a cut-off record
		}
		var k key
		copy(k[:], buf)
		codes := make([]int8, h.dims)
		for i := range codes {
			codes[i] = int8(buf[20+i])
		}
		each(k, record{math.Float32frombits(binary.LittleEndian.Uint32(buf[16:])), codes})
		n++
	}
}

// appender appends records to a cache file, creating it with a header.
type appender struct {
	f *os.File
	w *bufio.Writer
	h cacheHeader
}

// openAppend opens the cache for appending. fresh starts a new file
// (missing, foreign or damaged before).
func openAppend(path string, h cacheHeader, fresh bool) (*appender, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_APPEND
	if fresh {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return nil, err
	}
	a := &appender{f, bufio.NewWriter(f), h}
	if fresh {
		if err := h.write(a.w); err != nil {
			f.Close()
			return nil, err
		}
	}
	return a, nil
}

// add appends records and syncs them to disk: a checkpoint.
func (a *appender) add(keys []key, recs []record) error {
	for i := range keys {
		if err := a.h.writeRecord(a.w, keys[i], recs[i]); err != nil {
			return err
		}
	}
	if err := a.w.Flush(); err != nil {
		return err
	}
	return a.f.Sync()
}

func (a *appender) close() error { return a.f.Close() }

// readCacheCount is readCache without the header.
func readCacheCount(path, model string, each func(key, record)) (int, error) {
	_, n, err := readCache(path, model, each)
	return n, err
}

// compact rewrites the cache with only the records of the live keys, the
// last one of each, atomically: an interrupted rewrite leaves the old file.
func compact[V any](path, model string, live map[key]V) error {
	keep := map[key]record{}
	h, _, err := readCache(path, model, func(k key, r record) {
		if _, ok := live[k]; ok {
			keep[k] = r
		}
	})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".embeddings-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	w := bufio.NewWriterSize(tmp, 1<<20)
	err = h.write(w)
	for k, r := range keep {
		if err == nil {
			err = h.writeRecord(w, k, r)
		}
	}
	if err == nil {
		err = w.Flush()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// legacyJSON is the cache format before the binary one: text hash (hex of
// the same 16 bytes) → float32 unit vector.
type legacyJSON struct {
	Model   string               `json:"model"`
	Vectors map[string][]float32 `json:"vectors"`
}

// readLegacy reads the old JSON cache next to path (embeddings-m.json for
// embeddings-m.bin), so moving to the binary cache re-embeds nothing.
func readLegacy(path, model string, each func(key, record)) int {
	data, err := os.ReadFile(strings.TrimSuffix(path, ".bin") + ".json")
	if err != nil {
		return 0
	}
	var c legacyJSON
	if json.Unmarshal(data, &c) != nil || c.Model != model {
		return 0
	}
	n := 0
	for hexKey, v := range c.Vectors {
		var k key
		if b, err := hex.DecodeString(hexKey); err == nil && len(b) == len(k) {
			copy(k[:], b)
			codes, scale := quantize(normalize(v))
			each(k, record{scale, codes})
			n++
		}
	}
	return n
}
