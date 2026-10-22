package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fakeEmbed serves /embed with vectors on three topics (water, fire,
// other) by the words in each text, and /coin with fixed roots.
func fakeEmbed(t *testing.T) (*httptest.Server, *int) {
	texts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/embed":
			var req struct{ Texts []string }
			json.NewDecoder(r.Body).Decode(&req)
			texts += len(req.Texts)
			var vs [][]float32
			for _, s := range req.Texts {
				v := []float32{0, 0, 0.1}
				for _, w := range strings.Fields(strings.ToLower(s)) {
					switch {
					case slices.Contains([]string{"sea", "sea.", "water", "river", "lake"}, w):
						v[0]++
					case slices.Contains([]string{"fire", "light", "burn", "lantern"}, w):
						v[1]++
					}
				}
				vs = append(vs, v)
			}
			json.NewEncoder(w).Encode(map[string]any{"vectors": vs, "model": "m", "duration_ms": 5})
		case "/coin":
			fmt.Fprint(w, `{"coinage": {"definition": "A lamp.", "candidates": [{"proto": "pattuca", "reason": "fits"}]}, "prompt_tokens": 1, "eval_tokens": 1, "duration_ms": 1}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &texts
}

func TestSimilar(t *testing.T) {
	srv, texts := fakeEmbed(t)
	cache := filepath.Join(t.TempDir(), "emb.json")
	lnova := func(args ...string) (int, string, string) {
		var stdout, stderr bytes.Buffer
		code := run(append([]string{"-lang", "../../language", "similar", "-url", srv.URL, "-cache", cache}, args...), &stdout, &stderr)
		return code, stdout.String(), stderr.String()
	}

	code, out, errs := lnova("-k", "3", "-pos", "noun", "the", "sea")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	if !strings.Contains(errs, "embedded ") || !strings.Contains(errs, "(0 from the cache)") {
		t.Errorf("stderr = %q, want the embedding count", errs)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("%d lines, want 3:\n%s", len(lines), out)
	}
	for _, l := range lines {
		if !strings.Contains(l, "noun") {
			t.Errorf("line %q: want nouns only", l)
		}
	}
	if !strings.Contains(out, "water") {
		t.Errorf("stdout lacks water:\n%s", out)
	}

	// Second run: everything from the cache, only the query embedded.
	before := *texts
	if code, _, errs := lnova("sea"); code != 0 || strings.Contains(errs, "embedded") || *texts != before+1 {
		t.Errorf("cached run: exit %d, stderr %q, %d texts embedded", code, errs, *texts-before)
	}

	evalFile := filepath.Join(t.TempDir(), "e.yaml")
	os.WriteFile(evalFile, []byte("- query: the sea\n  expect: [sea, water, goat]\n"), 0o644)
	code, out, errs = lnova("-eval", evalFile)
	if code != 0 || !strings.Contains(out, "recall@10") || !strings.Contains(out, "2/3") || !strings.Contains(out, "goat") || !strings.Contains(out, "67%") {
		t.Errorf("eval: exit %d\n%s%s", code, out, errs)
	}

	for _, args := range [][]string{{}, {"-eval", evalFile, "sea"}, {"-pos", "thing", "sea"}} {
		if code, _, _ := lnova(args...); code != 2 {
			t.Errorf("lnova similar %v: exit %d, want 2", args, code)
		}
	}
	down := httptest.NewServer(http.NotFoundHandler())
	defer down.Close()
	var stderr bytes.Buffer
	if code := run([]string{"-lang", "../../language", "similar", "-url", down.URL, "-cache", "none", "sea"}, &bytes.Buffer{}, &stderr); code != 1 {
		t.Errorf("service down: exit %d, %s", code, stderr.String())
	}
}

func TestCoinRelated(t *testing.T) {
	srv, _ := fakeEmbed(t)
	cache := filepath.Join(t.TempDir(), "emb.json")
	var stdout, stderr bytes.Buffer
	code := run([]string{"-lang", "../../language", "coin", "-url", srv.URL, "-cache", cache, "-want", "1",
		"-pos", "noun", "-gender", "f", "lantern"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "related examples: ") || !strings.Contains(stderr.String(), "(fire)") {
		t.Errorf("stderr = %s\nwant the related examples, fire among them", stderr.String())
	}

	// No embedding service: a warning, and coin goes on.
	stderr.Reset()
	noEmbed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/embed" {
			http.NotFound(w, r)
			return
		}
		srv.Config.Handler.ServeHTTP(w, r)
	}))
	defer noEmbed.Close()
	code = run([]string{"-lang", "../../language", "coin", "-url", noEmbed.URL, "-cache", "none", "-want", "1",
		"-pos", "noun", "-gender", "f", "lantern"}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stderr.String(), "no related examples") {
		t.Errorf("no embed: exit %d, stderr %s", code, stderr.String())
	}
}
