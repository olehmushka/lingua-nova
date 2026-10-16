package main

import (
	"io"
	"net/http"
	"net/http/httptest"
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
		status int
		want   []string
		not    []string
	}{
		{"/", 200, []string{`name="q"`, "entries"}, nil},
		{"/?q=water", 200, []string{`href="/entry/water"`, "exa", "/ˈe.ʃa/", "gloss, exact"}, nil},
		{"/?q=*esia", 200, []string{`href="/entry/water"`, "proto, exact"}, nil},
		{"/?q=zzzz", 200, []string{"No entries match"}, nil},
		{"/?q=<b>", 200, []string{"&lt;b&gt;"}, []string{"<b>"}},
		{"/entry/water", 200, []string{"<h1>exa", "*esia", "si &gt; x / _V", "esia → exa", "nature.yaml"}, []string{`class="bad"`}},
		{"/entry/nope", 404, []string{"Not found"}, nil},
		{"/nowhere", 404, []string{"Not found"}, nil},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
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
