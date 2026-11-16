package eventtype_test

import (
	"strings"
	"testing"

	"github.com/blinklabs-io/bark/eventtype"
)

// TestRegisteredNamesAreUniqueAndCategorised checks that no name is listed
// twice and that every name has the category.name shape subscribers filter on.
func TestRegisteredNamesAreUniqueAndCategorised(t *testing.T) {
	seen := map[string]bool{}
	for _, name := range eventtype.Registered() {
		if seen[name] {
			t.Errorf("duplicate event type %q", name)
		}
		seen[name] = true
		category, rest, ok := strings.Cut(name, ".")
		if !ok || category == "" || rest == "" {
			t.Errorf("event type %q is not category.name", name)
		}
	}
	if len(seen) == 0 {
		t.Fatal("no event types registered")
	}
}

// TestIsRegistered checks a known name and the rollback case: Dingo carries a
// rollback inside chain.update, so a separate chain.rollback must not be
// advertised.
func TestIsRegistered(t *testing.T) {
	if !eventtype.IsRegistered(eventtype.ChainUpdate) {
		t.Error("chain.update should be registered")
	}
	if eventtype.IsRegistered("chain.rollback") {
		t.Error("chain.rollback is carried by chain.update, not registered")
	}
}

// TestRegisteredReturnsACopy checks that a caller changing the returned slice
// cannot alter the registry.
func TestRegisteredReturnsACopy(t *testing.T) {
	got := eventtype.Registered()
	if len(got) == 0 {
		t.Fatal("no event types registered")
	}
	got[0] = "tampered"
	after := eventtype.Registered()
	if len(after) == 0 || after[0] == "tampered" {
		t.Fatal("Registered exposed the internal slice")
	}
}

// TestInfosMatchRegistered checks that each description is filled in, its
// category is the name's prefix, and Lookup finds it.
func TestInfosMatchRegistered(t *testing.T) {
	names := eventtype.Registered()
	infos := eventtype.Infos()
	if len(infos) != len(names) {
		t.Fatalf("got %d infos for %d names", len(infos), len(names))
	}
	for i, info := range infos {
		if info.Name != names[i] {
			t.Errorf("info %d is %q, want %q", i, info.Name, names[i])
		}
		if prefix, _, _ := strings.Cut(info.Name, "."); info.Category != prefix {
			t.Errorf("%q has category %q, want %q", info.Name, info.Category, prefix)
		}
		if info.Description == "" {
			t.Errorf("%q has no description", info.Name)
		}
		if got, ok := eventtype.Lookup(info.Name); !ok || got != info {
			t.Errorf("Lookup(%q) = %v, %v", info.Name, got, ok)
		}
	}
	if _, ok := eventtype.Lookup("chain.rollback"); ok {
		t.Error("Lookup found an unregistered type")
	}
}
