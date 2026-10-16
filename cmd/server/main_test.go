package main

import (
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPages(t *testing.T) {
	h, err := newHandler("../../language")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		path   string
		htmx   bool // sent by htmx: expects just the results fragment
		status int
		want   []string
		not    []string
	}{
		{"/", false, 200, []string{`name="q"`, "entries"}, nil},
		{"/?q=water", false, 200, []string{`href="/entry/water"`, "exa", "/ˈe.ʃa/", "gloss, exact"}, nil},
		{"/?q=*esia", false, 200, []string{`href="/entry/water"`, "proto, exact"}, nil},
		{"/?q=zzzz", false, 200, []string{"No entries match"}, nil},
		{"/?q=<b>", false, 200, []string{"&lt;b&gt;"}, []string{"<b>"}},
		{"/entry/water", false, 200, []string{"<h1>exa", "*esia", "si &gt; x / _V", "esia → exa", "nature.yaml"}, []string{`class="bad"`}},
		{"/entry/nope", false, 404, []string{"Not found"}, nil},
		{"/nowhere", false, 404, []string{"Not found"}, nil},
		{"/?q=water", false, 200, []string{`<div id="results">`, `hx-get="/"`, "/static/htmx.min.js"}, nil},
		{"/?q=water", true, 200, []string{`href="/entry/water"`, "gloss, exact"}, []string{"<!doctype", "<header>", `name="q"`}},
		{"/?q=", true, 200, []string{"entries. Search"}, []string{"<!doctype"}},
		{"/?q=<zzzz>", true, 200, []string{"No entries match", "&lt;zzzz&gt;"}, []string{"<zzzz>"}},
		{"/static/htmx.min.js", false, 200, []string{"htmx"}, nil},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		if tt.htmx {
			req.Header.Set("HX-Request", "true")
		}
		h.ServeHTTP(rec, req)
		body, _ := io.ReadAll(rec.Body)
		if rec.Code != tt.status {
			t.Errorf("GET %s: status %d, want %d", tt.path, rec.Code, tt.status)
		}
		for _, w := range tt.want {
			if !strings.Contains(string(body), w) {
				t.Errorf("GET %s: body lacks %q", tt.path, w)
			}
		}
		for _, n := range tt.not {
			if strings.Contains(string(body), n) {
				t.Errorf("GET %s: body contains %q", tt.path, n)
			}
		}
	}
}

// copyLang copies the real language/ into a temp dir the test can edit.
func copyLang(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("../../language")); err != nil {
		t.Fatal(err)
	}
	return dir
}

func get(t *testing.T, h http.Handler, path string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Body.String()
}

func TestReload(t *testing.T) {
	dir := copyLang(t)
	s, err := newHandler(dir)
	if err != nil {
		t.Fatal(err)
	}
	verbs := filepath.Join(dir, "lexicon", "verbs.yaml")
	orig, err := os.ReadFile(verbs)
	if err != nil {
		t.Fatal(err)
	}
	const newVerb = `
- id: zorp
  proto: zorpar
  modern: zorpar
  pos: verb
  gloss: zorp
  definition: "A test verb."
`
	const badVerb = `
- id: zorpo
  proto: zorpo
  modern: zorpo
  pos: verb
  gloss: zorpo
  definition: "A verb without a verb ending."
`
	steps := []struct {
		name string
		file string // new verbs.yaml content
		want []string
		not  []string
	}{
		{"add entry", string(orig) + newVerb,
			[]string{`href="/entry/zorp"`}, []string{`class="banner`}},
		{"broken yaml keeps old data", string(orig) + newVerb + "- id: [\n",
			[]string{`href="/entry/zorp"`, `class="banner banner-error"`, "verbs.yaml"}, []string{`class="banner banner-warn"`}},
		{"fixed", string(orig) + newVerb,
			[]string{`href="/entry/zorp"`}, []string{`class="banner`}},
		{"validate problem shows new data", string(orig) + newVerb + badVerb,
			[]string{`href="/entry/zorpo"`, `class="banner banner-warn"`, "verbs.yaml:", "zorpo: verb"}, []string{`class="banner banner-error"`}},
	}
	for _, st := range steps {
		if err := os.WriteFile(verbs, []byte(st.file), 0o644); err != nil {
			t.Fatal(err)
		}
		s.reload()
		body := get(t, s, "/?q=zorp")
		for _, w := range st.want {
			if !strings.Contains(body, w) {
				t.Errorf("%s: body lacks %q", st.name, w)
			}
		}
		for _, n := range st.not {
			if strings.Contains(body, n) {
				t.Errorf("%s: body contains %q", st.name, n)
			}
		}
	}
}

func TestStamps(t *testing.T) {
	dir := copyLang(t)
	extra := filepath.Join(dir, "lexicon", "extra.yaml")
	edits := []struct {
		name string
		edit func() error
	}{
		{"append", func() error {
			f, err := os.OpenFile(filepath.Join(dir, "sound-changes.txt"), os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = f.WriteString("\n")
			return err
		}},
		{"new file", func() error { return os.WriteFile(extra, nil, 0o644) }},
		{"delete file", func() error { return os.Remove(extra) }},
	}
	prev := stamps(dir)
	if len(prev) < 3 {
		t.Fatalf("stamps found %d files", len(prev))
	}
	for _, e := range edits {
		if err := e.edit(); err != nil {
			t.Fatal(err)
		}
		cur := stamps(dir)
		if maps.Equal(prev, cur) {
			t.Errorf("%s: stamps unchanged", e.name)
		}
		prev = cur
	}
}
