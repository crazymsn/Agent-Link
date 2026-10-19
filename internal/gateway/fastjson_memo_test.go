package gateway

import (
	"bytes"
	"strings"
	"sync/atomic"
	"testing"
)

// memoBody is a request body past validMemoMin, unique to the test.
func memoBody(tag string) []byte {
	return []byte(`{"model":"` + tag + `","messages":[{"role":"user","content":"` +
		strings.Repeat("a long prompt ", 6000) + `"}]}`)
}

// countScans counts jsonValid's byte-by-byte checks of bodies equal to b.
func countScans(t *testing.T, b []byte) *atomic.Int32 {
	var n atomic.Int32
	f := func(data []byte) {
		if bytes.Equal(data, b) {
			n.Add(1)
		}
	}
	validScanned.Store(&f)
	t.Cleanup(func() { validScanned.Store(nil) })
	return &n
}

// A long body decoded by each reader of a request is checked for valid
// JSON once, not by every decodeJSON: at 2 MB each check cost ~2.5ms, and
// a request's body went through a dozen of them.
func TestDecodeJSONChecksALongBodyOnce(t *testing.T) {
	body := memoBody(t.Name())
	scans := countScans(t, body)
	for range 6 {
		var v struct {
			Model string `json:"model"`
		}
		if err := decodeJSON(body, &v); err != nil || v.Model != t.Name() {
			t.Fatalf("decode: %v %q", err, v.Model)
		}
	}
	if n := scans.Load(); n != 1 {
		t.Fatalf("a %d-byte body was checked %d times over six decodes, want 1", len(body), n)
	}
}

// What is remembered is a valid body: an invalid one, or a valid one
// changed by a byte at the same length, is checked again and refused.
func TestDecodeJSONMemoKeepsRefusing(t *testing.T) {
	body := memoBody(t.Name())
	var v map[string]any
	if err := decodeJSON(body, &v); err != nil {
		t.Fatal(err)
	}
	bad := bytes.Clone(body)
	bad[len(bad)-1] = '!'
	for range 3 {
		var w map[string]any
		if err := decodeJSON(bad, &w); err == nil {
			t.Fatal("a body with a broken end decoded")
		}
	}
	trunc := body[:len(body)-1]
	var w map[string]any
	if err := decodeJSON(trunc, &w); err == nil {
		t.Fatal("a truncated body decoded")
	}
	if !jsonValid(body) || jsonValid(bad) {
		t.Fatal("jsonValid's answer changed")
	}
}
