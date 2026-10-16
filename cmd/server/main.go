// Command server is the Rifatian dictionary web app: search in both
// directions, an entry page with IPA and the sound-change derivation, and
// an interlinear glosser.
//
//	go run ./cmd/server [-lang language] [-addr :8080] [-poll 500ms]
//
// It reloads language/ when a file changes. If the new files don't load,
// it keeps serving the old data and shows the error on every page.
package main

import (
	"bytes"
	"cmp"
	"embed"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/olehmushka/lingua-nova/internal/gloss"
	"github.com/olehmushka/lingua-nova/internal/lexicon"
	"github.com/olehmushka/lingua-nova/internal/morph"
	"github.com/olehmushka/lingua-nova/internal/phon"
	"github.com/olehmushka/lingua-nova/internal/sound"
)

//go:embed templates/*.html
var templateFS embed.FS

// static holds htmx.min.js (2.0.4), served at /static/ so the app works
// offline.
//
//go:embed static
var staticFS embed.FS

func main() {
	langDir := flag.String("lang", "language", "language data directory")
	addr := flag.String("addr", ":8080", "listen address")
	poll := flag.Duration("poll", 500*time.Millisecond, "how often to check language/ for changes (0 = never)")
	flag.Parse()

	s, err := newHandler(*langDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "server: %v\n", err)
		os.Exit(1)
	}
	if *poll > 0 {
		go s.watch(*poll)
	}
	host := *addr
	if strings.HasPrefix(host, ":") {
		host = "localhost" + host
	}
	log.Printf("dictionary on http://%s", host)
	log.Fatal(http.ListenAndServe(*addr, s))
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
	Paradigm   *morph.Table     // nil if the word doesn't inflect
}

// ParadigmTitle names the paradigm table: verbs conjugate, nouns decline.
func (e *entryInfo) ParadigmTitle() string {
	switch e.POS {
	case "verb":
		return "Conjugation"
	case "noun":
		return "Declension"
	}
	return "Forms"
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
	langDir string
	state   atomic.Pointer[state]
	pages   map[string]*template.Template
	mux     *http.ServeMux
}

// state is what the pages read: the data from the last good load, plus the
// error from the last load if it failed. It's never modified after it's
// stored; reload builds a new one and swaps the pointer, so a request that
// calls state.Load once sees one consistent snapshot.
type state struct {
	d   *data
	err error // last reload failed; d is from before it
}

// data is everything computed from language/.
type data struct {
	lex      *lexicon.Lexicon
	info     map[string]*entryInfo // by id
	analyzer *morph.Analyzer
	glosser  *gloss.Glosser
	problems []lexicon.Problem // what lnova validate reports
}

// newHandler loads the language data and returns the app's routes.
func newHandler(langDir string) (*server, error) {
	d, err := load(langDir)
	if err != nil {
		return nil, err
	}
	s := &server{langDir: langDir, pages: map[string]*template.Template{}}
	s.state.Store(&state{d: d})

	for _, page := range []string{"search", "entry", "gloss", "notfound"} {
		t, err := template.ParseFS(templateFS, "templates/base.html", "templates/"+page+".html")
		if err != nil {
			return nil, err
		}
		s.pages[page] = t
	}

	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /{$}", s.handleSearch)
	s.mux.HandleFunc("GET /entry/{id}", s.handleEntry)
	s.mux.HandleFunc("GET /gloss", s.handleGloss)
	static, _ := fs.Sub(staticFS, "static")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.render(w, http.StatusNotFound, "notfound", r.URL.Path)
	})
	return s, nil
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

// load reads language/ and computes everything the pages show. An error
// means the files can't be used at all (bad YAML, a rule that won't parse);
// content problems are in data.problems.
func load(langDir string) (*data, error) {
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
	morphRules, err := morph.Load(filepath.Join(langDir, "morphology.yaml"), inv)
	if err != nil {
		return nil, err
	}

	probs := append(lexicon.Validate(lex, inv), morph.Check(lex, morphRules)...)
	slices.SortStableFunc(probs, func(a, b lexicon.Problem) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
	})
	analyzer := morph.NewAnalyzer(morphRules, lex)
	d := &data{lex: lex, info: map[string]*entryInfo{}, analyzer: analyzer, glosser: gloss.New(analyzer), problems: probs}
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
		// A word that can't be inflected (a verb without a class ending)
		// is a validate problem; the page just has no table.
		info.Paradigm, _ = morphRules.Paradigm(e)
		for _, h := range byModern[e.Modern] {
			if h != e {
				info.Homophones = append(info.Homophones, h)
			}
		}
		// Duplicate ids are a validate problem; the first one wins, as in
		// the lexicon order.
		if _, dup := d.info[e.ID]; !dup {
			d.info[e.ID] = info
		}
	}
	return d, nil
}

// reload loads language/ again. On success the new data replaces the old;
// on failure the old data stays and the error is shown on every page.
func (s *server) reload() {
	d, err := load(s.langDir)
	if err != nil {
		s.state.Store(&state{d: s.state.Load().d, err: err})
		log.Printf("reload failed, keeping old data: %v", err)
		return
	}
	s.state.Store(&state{d: d})
	noun := "problems"
	if len(d.problems) == 1 {
		noun = "problem"
	}
	log.Printf("reloaded: %d entries, %d %s", len(d.lex.Entries), len(d.problems), noun)
}

// stamp is what the watcher compares to notice that a file changed.
type stamp struct {
	size int64
	mod  time.Time
}

// stamps returns the size and modification time of every file load reads.
// The lexicon files come from a glob, so a new or deleted file changes the
// map too. A file that can't be stat'ed is left out; load reports it.
func stamps(langDir string) map[string]stamp {
	files, _ := filepath.Glob(filepath.Join(langDir, "lexicon", "*.yaml"))
	for _, f := range []string{"phonology.yaml", "sound-changes.txt", "morphology.yaml"} {
		files = append(files, filepath.Join(langDir, f))
	}
	m := map[string]stamp{}
	for _, f := range files {
		if fi, err := os.Stat(f); err == nil {
			m[f] = stamp{fi.Size(), fi.ModTime()}
		}
	}
	return m
}

// watch polls the files every interval and reloads when any of them
// changed. Polling is a few stat calls per tick, needs nothing outside the
// standard library, and doesn't care how an editor saves (in place, or a
// temp file renamed over the original).
func (s *server) watch(interval time.Duration) {
	prev := stamps(s.langDir)
	for range time.Tick(interval) {
		cur := stamps(s.langDir)
		if !maps.Equal(prev, cur) {
			s.reload()
			prev = cur
		}
	}
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

// form is a search query read as an inflected form of an entry.
type form struct {
	*entryInfo
	Form morph.Form // named, so .Gloss stays the entry's gloss
}

type result struct {
	*entryInfo
	Field string
	Match lexicon.Match
}

func (s *server) handleSearch(w http.ResponseWriter, r *http.Request) {
	d := s.state.Load().d
	q := r.URL.Query().Get("q")
	var results []result
	for _, h := range d.lex.Search(q) {
		results = append(results, result{d.info[h.Entry.ID], h.Field, h.Match})
	}
	// A one-word query may be an inflected form: naraxu → see. Guesses
	// about roots not in the lexicon are left to lnova analyze.
	var forms []form
	if w := strings.TrimSpace(q); w != "" && !strings.ContainsAny(w, " *") {
		as, _ := d.analyzer.Analyze(w)
		for _, a := range as {
			if !a.Guess() {
				forms = append(forms, form{d.info[a.Entry.ID], a.Form})
			}
		}
	}
	data := map[string]any{
		"Query":   q,
		"Forms":   forms,
		"Results": results,
		"Total":   len(d.lex.Entries),
	}
	// Live search: htmx sends HX-Request and swaps the response into
	// #results, so it gets just that fragment. Vary tells caches the same
	// URL has two shapes.
	w.Header().Add("Vary", "HX-Request")
	if r.Header.Get("HX-Request") == "true" {
		s.execute(w, http.StatusOK, "search", "results", data)
		return
	}
	s.render(w, http.StatusOK, "search", data)
}

// handleGloss glosses the sentence in ?s, live like search: htmx gets
// just the gloss fragment.
func (s *server) handleGloss(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("s")
	data := map[string]any{"Sentence": q}
	if strings.TrimSpace(q) != "" {
		data["Result"] = s.state.Load().d.glosser.Gloss(q)
	}
	w.Header().Add("Vary", "HX-Request")
	if r.Header.Get("HX-Request") == "true" {
		s.execute(w, http.StatusOK, "gloss", "gloss", data)
		return
	}
	s.render(w, http.StatusOK, "gloss", data)
}

func (s *server) handleEntry(w http.ResponseWriter, r *http.Request) {
	info, ok := s.state.Load().d.info[r.PathValue("id")]
	if !ok {
		s.render(w, http.StatusNotFound, "notfound", r.URL.Path)
		return
	}
	s.render(w, http.StatusOK, "entry", info)
}

// view is what base.html gets: the page's own data, plus the load status
// for the banner.
type view struct {
	Data     any
	Err      error             // last reload failed
	Problems []lexicon.Problem // first maxProblems validate problems
	More     int               // how many problems aren't listed
}

const maxProblems = 10

// render executes a whole page, with the banner.
func (s *server) render(w http.ResponseWriter, status int, page string, data any) {
	st := s.state.Load()
	v := view{Data: data, Err: st.err, Problems: st.d.problems}
	if len(v.Problems) > maxProblems {
		v.More = len(v.Problems) - maxProblems
		v.Problems = v.Problems[:maxProblems]
	}
	s.execute(w, status, page, "base", v)
}

// execute runs one named template of a page into a buffer first, so a
// template error gives a clean 500 instead of half a page.
func (s *server) execute(w http.ResponseWriter, status int, page, name string, data any) {
	var buf bytes.Buffer
	if err := s.pages[page].ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("render %s/%s: %v", page, name, err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	buf.WriteTo(w)
}
