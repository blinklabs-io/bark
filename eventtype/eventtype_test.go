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
