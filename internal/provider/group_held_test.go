package provider

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/crazymsn/Agent-Link/internal/settings"
)

// groupHeldFixture is a dozen providers serving the same models, so
// agentlink finds a group of each, and a group of the user's of four.
func groupHeldFixture(t *testing.T) {
	t.Helper()
	isolate(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("AGENT_LINK_NO_ACCOUNTS", "1")
	var models []string
	for i := range 10 {
		models = append(models, fmt.Sprintf("m%d", i))
	}
	for i := range 12 {
		if err := Save(Provider{ID: fmt.Sprintf("p%d", i), Name: fmt.Sprintf("P%d", i), Key: "k", Models: models, Anthropic: "http://127.0.0.1:9"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := settings.Save(settings.Settings{DisabledModels: []string{"p11/m9"}}); err != nil {
		t.Fatal(err)
	}
	if err := SaveGroup(Group{ID: "g4", Name: "G4", Members: []string{"p0/m1", "p1/m1", "p2/m1", "p3/m1"}, Routing: Ordered}); err != nil {
		t.Fatal(err)
	}
	Changed()
}

// A request to a group asks for the catalog's groups several times —
// Resolve's Catalog, and Served for the members' images, the reply's
// length and the window — and each built every group's entry again, from
// every member of every group: the burst of CPU at once that a request to
// a group cost with many providers (colmugx and Yayoi_no_yume on X). While
// a request holds the catalog they are built once.
func TestGroupEntriesBuiltOnceWhileHeld(t *testing.T) {
	groupHeldFixture(t)
	release := Hold()
	defer release()
	entries := providerEntries()
	first := groupEntries(entries)
	if len(first) < 2 {
		t.Fatalf("the fixture has %d groups; the test needs the found ones too", len(first))
	}
	if again := groupEntries(entries); &again[0] != &first[0] {
		t.Fatal("the groups' entries were built again under one Hold")
	}
	// what a caller is given is its own to change
	c := Catalog()
	c[0].Name = "changed"
	if Catalog()[0].Name == "changed" || Served()[0].Name == "changed" {
		t.Fatal("a change to Catalog's entries reached the next caller's")
	}
	// another catalog than the one held is built from, not answered for
	other := append([]Entry(nil), entries[:len(entries)/2]...)
	if got := groupEntries(other); len(got) > 0 && &got[0] == &first[0] {
		t.Fatal("the groups of another catalog were the held one's")
	}
	// a write drops them
	if err := SaveGroup(Group{ID: "g4", Name: "Renamed", Members: []string{"p0/m1", "p1/m1"}, Routing: Ordered}); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range Catalog() {
		found = found || e.ID == GroupPrefix+"g4" && e.Name == "Renamed"
	}
	if !found {
		t.Fatal("a group saved under the Hold kept its old entry")
	}
}

// Each group's members (membersIn) asked whether the user disabled them,
// and that parsed settings.json each time: a catalog of found groups
// parsed it once a group, on each of the several looks a request makes.
func TestGroupLooksParseNoSettingsPerGroupWhileHeld(t *testing.T) {
	groupHeldFixture(t)
	release := Hold()
	defer release()
	Served() // the first look builds what is held
	const looks = 50
	before := settings.Parses()
	for range looks {
		Served()
		Groups()
		if _, _, ok := FindGroup(GroupPrefix + "g4"); !ok {
			t.Fatal("no group g4")
		}
	}
	if n := settings.Parses() - before; n >= looks {
		t.Fatalf("settings.json parsed %d times in %d looks at the groups under one Hold", n, looks)
	}
	// and a model disabled is still left out
	for _, e := range Served() {
		if e.ID == "p11/m9" {
			t.Fatal("a disabled model was served")
		}
	}
}

// BenchmarkGroupEntries is the groups' entries of a catalog of 30
// providers serving the same 60 models (60 groups agentlink finds, of 30
// members each), built afresh: what each request built several times.
func BenchmarkGroupEntries(b *testing.B) {
	isolate(b)
	home := b.TempDir()
	b.Setenv("HOME", home)
	b.Setenv("USERPROFILE", home)
	b.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	b.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	b.Setenv("AGENT_LINK_NO_ACCOUNTS", "1")
	var models []string
	for i := range 60 {
		models = append(models, fmt.Sprintf("m%d", i))
	}
	for i := range 30 {
		if err := Save(Provider{ID: fmt.Sprintf("p%d", i), Name: fmt.Sprintf("P%d", i), Key: "k", Models: models, Anthropic: "http://127.0.0.1:9"}); err != nil {
			b.Fatal(err)
		}
	}
	Changed()
	entries := providerEntries()
	b.ResetTimer()
	for range b.N {
		buildGroupEntries(entries)
	}
}
