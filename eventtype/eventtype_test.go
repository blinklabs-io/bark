package eventtype_test

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/blinklabs-io/bark/eventtype"
	eventv1alpha1 "github.com/blinklabs-io/bark/proto/v1alpha1/event"
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

// TestIsRegistered checks the types a subscriber filters on, including the
// ones with their own name rather than a shared chain.update.
func TestIsRegistered(t *testing.T) {
	for _, name := range []string{
		eventtype.ChainUpdate,
		eventtype.ChainRollback,
		eventtype.LedgerSyncStateChanged,
		eventtype.LedgerEpochTransitioned,
		eventtype.LedgerProtocolParamsUpdate,
		eventtype.DatabaseOperation,
	} {
		if !eventtype.IsRegistered(name) {
			t.Errorf("%s should be registered", name)
		}
	}
	if eventtype.IsRegistered("epoch.transition") {
		t.Error("epoch.transition is an internal name, not registered")
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
	if _, ok := eventtype.Lookup("chain.nothing"); ok {
		t.Error("Lookup found an unregistered type")
	}
}

// TestTypedPayloadsResolveAndDecode checks that every type reporting a typed
// payload names a message whose descriptors are self-contained, and that a
// type without one is refused.
func TestTypedPayloadsResolveAndDecode(t *testing.T) {
	typedCount := 0
	for _, info := range eventtype.Infos() {
		set, err := eventtype.PayloadDescriptors(info.Name)
		if !info.HasTypedPayload {
			if err == nil || info.PayloadMessage != "" {
				t.Errorf("%s: no payload expected, got err=%v", info.Name, err)
			}
			continue
		}
		typedCount++
		if err != nil {
			t.Errorf("%s: %v", info.Name, err)
			continue
		}
		files, err := protodesc.NewFiles(set)
		if err != nil {
			t.Errorf("%s: descriptors are not self-contained: %v", info.Name, err)
			continue
		}
		if _, err := files.FindDescriptorByName(
			protoreflect.FullName(info.PayloadMessage),
		); err != nil {
			t.Errorf("%s: %s not in descriptors: %v", info.Name, info.PayloadMessage, err)
		}
	}
	if typedCount == 0 {
		t.Fatal("no event type has a typed payload")
	}
}

// TestPayloadRoundTripsThroughAny checks a payload packs into the envelope's
// typed_data and unpacks to the message the registry names.
func TestPayloadRoundTripsThroughAny(t *testing.T) {
	want := &eventv1alpha1.PeerChurnEventData{
		Address: "10.0.0.1:3001", OldState: "hot", NewState: "warm", Score: 0.5,
	}
	any, err := anypb.New(want)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := eventtype.Lookup(eventtype.PeerChurn)
	if string(any.MessageName()) != info.PayloadMessage {
		t.Fatalf("registry names %q, packed %q", info.PayloadMessage, any.MessageName())
	}
	got := &eventv1alpha1.PeerChurnEventData{}
	if err := any.UnmarshalTo(got); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestPayloadDescriptorsRejectsUnregistered checks the two refusals.
func TestPayloadDescriptorsRejectsUnregistered(t *testing.T) {
	if _, err := eventtype.PayloadDescriptors("nope.nothing"); err == nil {
		t.Error("unregistered type should fail")
	}
	if _, err := eventtype.PayloadDescriptors(eventtype.LedgerError); err == nil {
		t.Error("type without payload should fail")
	}
}
