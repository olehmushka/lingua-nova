// Command server is the Rifatian dictionary web app: search in both
// directions, and an entry page with IPA and the sound-change derivation.
//
//	go run ./cmd/server [-lang language] [-addr :8080]
//
// It reads language/ once at start; restart it after editing the files.
package main

import (
	"bytes"
	"embed"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
	"github.com/olehmushka/lingua-nova/internal/phon"
	"github.com/olehmushka/lingua-nova/internal/sound"
)

//go:embed templates/*.html
var templateFS embed.FS

func main() {
	langDir := flag.String("lang", "language", "language data directory")
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	h, err := newHandler(*langDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "server: %v\n", err)
		os.Exit(1)
	}
	host := *addr
	if strings.HasPrefix(host, ":") {
		host = "localhost" + host
	}
	log.Printf("dictionary on http://%s", host)
	log.Fatal(http.ListenAndServe(*addr, h))
}

// entryInfo is what the pages show about an entry beyond its YAML fields,
// computed once at load time.
type entryInfo struct {
	*lexicon.Entry
	Headword   string // word if set, else modern
	IPA        string // modern, syllabified and stressed: naˈɾa.ʃu
	ProtoIPA   string
	Evolved    string // what the sound changes give from proto
	Steps      []step
	Homophones []*lexicon.Entry // other entries with the same modern form
}

// Mismatch reports whether the sound changes don't reproduce modern, the
// same check as lnova evolve.
func (e *entryInfo) Mismatch() bool { return e.Evolved != e.Modern }

// step is one line of a derivation, already spelled out for the template.
type step struct {
	Line          int
	Rule          string
	Before, After string
}

type server struct {
	lex   *lexicon.Lexicon
	info  map[string]*entryInfo // by id
	pages map[string]*template.Template
}

// newHandler loads the language data and returns the app's routes.
func newHandler(langDir string) (http.Handler, error) {
	inv, err := phon.Load(filepath.Join(langDir, "phonology.yaml"))
	if err != nil {
		return nil, err
	}
	rules, err := sound.Load(filepath.Join(langDir, "sound-changes.txt"), inv)
	if err != nil {
		return nil, err
	}
	lex, err := lexicon.Load(filepath.Join(langDir, "lexicon"))
	if err != nil {
		return nil, err
	}

	s := &server{lex: lex, info: map[string]*entryInfo{}, pages: map[string]*template.Template{}}
	byModern := map[string][]*lexicon.Entry{}
	for _, e := range lex.Entries {
		byModern[e.Modern] = append(byModern[e.Modern], e)
	}
	for _, e := range lex.Entries {
		proto := strings.TrimPrefix(e.Proto, "*")
		info := &entryInfo{Entry: e, Headword: e.Modern, IPA: ipa(inv, e.Modern), ProtoIPA: ipa(inv, proto)}
		if e.Word != "" {
			info.Headword = e.Word
		}
		// A proto form with an unknown letter has no derivation; lnova
		// validate reports it, and the page shows an empty result.
		if segs, err := inv.Tokenize(proto); err == nil {
			out, steps := sound.Evolve(inv, rules, segs)
			info.Evolved = phon.Spell(out)
			for _, st := range steps {
				info.Steps = append(info.Steps, step{st.Rule.Line, st.Rule.Text, phon.Spell(st.Before), phon.Spell(st.After)})
			}
		}
		for _, h := range byModern[e.Modern] {
			if h != e {
				info.Homophones = append(info.Homophones, h)
			}
		}
		s.info[e.ID] = info
	}

	for _, page := range []string{"search", "entry", "notfound"} {
		t, err := template.ParseFS(templateFS, "templates/base.html", "templates/"+page+".html")
		if err != nil {
			return nil, err
		}
		s.pages[page] = t
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleSearch)
	mux.HandleFunc("GET /entry/{id}", s.handleEntry)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.render(w, http.StatusNotFound, "notfound", r.URL.Path)
	})
	return mux, nil
}

// ipa returns a word's syllabified, stressed IPA, or "" if it can't be
// parsed (lnova validate reports those).
func ipa(inv *phon.Inventory, word string) string {
	segs, err := inv.Tokenize(word)
	if err != nil {
		return ""
	}
	syls, err := inv.Syllabify(segs)
	if err != nil {
		return ""
	}
	return phon.SyllableIPA(syls, phon.Stress(syls))
}

type result struct {
	*entryInfo
	Field string
	Match lexicon.Match
}

func (s *server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	var results []result
	for _, h := range s.lex.Search(q) {
		results = append(results, result{s.info[h.Entry.ID], h.Field, h.Match})
	}
	s.render(w, http.StatusOK, "search", map[string]any{
		"Query":   q,
		"Results": results,
		"Total":   len(s.lex.Entries),
	})
}

func (s *server) handleEntry(w http.ResponseWriter, r *http.Request) {
	info, ok := s.info[r.PathValue("id")]
	if !ok {
		s.render(w, http.StatusNotFound, "notfound", r.URL.Path)
		return
	}
	s.render(w, http.StatusOK, "entry", info)
}

// render executes a page into a buffer first, so a template error gives a
// clean 500 instead of half a page.
func (s *server) render(w http.ResponseWriter, status int, page string, data any) {
	var buf bytes.Buffer
	if err := s.pages[page].ExecuteTemplate(&buf, "base", data); err != nil {
		log.Printf("render %s: %v", page, err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	buf.WriteTo(w)
}
