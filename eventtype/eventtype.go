// Package eventtype lists the event types a Dingo node exposes through the
// EventService. The names are the exact strings the node's EventBus uses, so
// a type registered here is one the node emits.
package eventtype

import (
	"fmt"
	"slices"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	eventv1alpha1 "github.com/blinklabs-io/bark/proto/v1alpha1/event"
)

// Peer governance events.
const (
	PeerChurn         = "peergov.peer_churn"
	BootstrapExited   = "peergov.bootstrap_exited"
	BootstrapRecovery = "peergov.bootstrap_recovery"
	QuotaStatus       = "peergov.quota_status"
	OutboundConn      = "peergov.outbound_conn"
)

// Chain events.
const (
	ChainUpdate       = "chain.update"
	ChainRollback     = "chain.rollback"
	ChainForkDetected = "chain.fork_detected"
)

// Ledger events.
const (
	LedgerSyncStateChanged     = "ledger.sync_state_changed"
	LedgerEpochTransitioned    = "ledger.epoch_transitioned"
	LedgerProtocolParamsUpdate = "ledger.protocol_params_updated"
	LedgerError                = "ledger.error"
	LedgerBlockfetch           = "ledger.blockfetch"
	LedgerChainsync            = "ledger.chainsync"
)

// Database events.
const (
	DatabaseOperation = "database.operation"
)

// Mempool events.
const (
	MempoolAddTx    = "mempool.add_tx"
	MempoolRemoveTx = "mempool.remove_tx"
)

// Connection manager events.
const (
	ConnManagerInboundConn = "connmanager.inbound_conn"
	ConnManagerConnClosed  = "connmanager.conn_closed"
)

// Info describes one registered event type.
type Info struct {
	// Name is the event type name, such as "chain.update".
	Name string
	// Category is the subsystem that emits the type: the part of Name
	// before the first dot.
	Category string
	// Description says when the event is emitted.
	Description string
	// HasTypedPayload reports whether events of this type carry a typed
	// payload in Event.typed_data.
	HasTypedPayload bool
	// PayloadMessage is the fully qualified name of the message packed into
	// Event.typed_data. Empty when HasTypedPayload is false.
	PayloadMessage string
}

func info(name, category, description string) Info {
	return Info{Name: name, Category: category, Description: description}
}

func typed(
	name, category, description string,
	payload proto.Message,
) Info {
	return Info{
		Name:            name,
		Category:        category,
		Description:     description,
		HasTypedPayload: true,
		PayloadMessage:  string(payload.ProtoReflect().Descriptor().FullName()),
	}
}

var registered = []Info{
	typed(PeerChurn, "peergov", "A peer was replaced or changed state in peer governance.", &eventv1alpha1.PeerChurnEventData{}),
	info(BootstrapExited, "peergov", "The node stopped using its bootstrap peers."),
	info(BootstrapRecovery, "peergov", "The node returned to its bootstrap peers."),
	info(QuotaStatus, "peergov", "Peer governance reported its peer quota status."),
	info(OutboundConn, "peergov", "An outbound connection to a peer was made."),
	typed(ChainUpdate, "chain", "A block was added to the chain.", &eventv1alpha1.ChainUpdateEventData{}),
	typed(ChainRollback, "chain", "The chain rolled back to an earlier point.", &eventv1alpha1.ChainRollbackEventData{}),
	info(ChainForkDetected, "chain", "A chain fork was detected."),
	typed(LedgerSyncStateChanged, "ledger", "The ledger's chain sync state changed.", &eventv1alpha1.SyncStateEventData{}),
	typed(LedgerEpochTransitioned, "ledger", "The ledger crossed an epoch boundary.", &eventv1alpha1.EpochTransitionEventData{}),
	info(LedgerProtocolParamsUpdate, "ledger", "The protocol parameters changed."),
	info(LedgerError, "ledger", "The ledger reported an error."),
	info(LedgerBlockfetch, "ledger", "A blockfetch protocol event reached the ledger."),
	info(LedgerChainsync, "ledger", "A chainsync protocol event reached the ledger."),
	typed(MempoolAddTx, "mempool", "A transaction was added to the mempool.", &eventv1alpha1.TransactionEventData{}),
	typed(MempoolRemoveTx, "mempool", "A transaction was removed from the mempool.", &eventv1alpha1.TransactionEventData{}),
	typed(ConnManagerInboundConn, "connmanager", "An inbound connection was accepted.", &eventv1alpha1.ConnectionEventData{}),
	typed(ConnManagerConnClosed, "connmanager", "A connection was closed.", &eventv1alpha1.ConnectionEventData{}),
	typed(DatabaseOperation, "database", "A database operation changed state.", &eventv1alpha1.DatabaseOperationEventData{}),
}

// Registered returns the name of every registered event type. The caller may
// modify the returned slice.
func Registered() []string {
	names := make([]string, 0, len(registered))
	for _, e := range registered {
		names = append(names, e.Name)
	}
	return names
}

// Infos returns a description of every registered event type, in the order
// of Registered. The caller may modify the returned slice.
func Infos() []Info {
	return slices.Clone(registered)
}

// Lookup returns the description of the named event type.
func Lookup(name string) (Info, bool) {
	for _, e := range registered {
		if e.Name == name {
			return e, true
		}
	}
	return Info{}, false
}

// IsRegistered reports whether name is a registered event type.
func IsRegistered(name string) bool {
	_, ok := Lookup(name)
	return ok
}

// PayloadDescriptors returns the definitions of the named event type's typed
// payload message and every file it depends on, in dependency order, so a
// client can decode the payload without compiled-in knowledge of it. It
// returns an error when the type is unregistered or has no typed payload.
func PayloadDescriptors(
	name string,
) (*descriptorpb.FileDescriptorSet, error) {
	info, ok := Lookup(name)
	if !ok {
		return nil, fmt.Errorf("event type %q is not registered", name)
	}
	if !info.HasTypedPayload {
		return nil, fmt.Errorf("event type %q has no typed payload", name)
	}
	msg, err := protoregistry.GlobalTypes.FindMessageByName(
		protoreflect.FullName(info.PayloadMessage),
	)
	if err != nil {
		return nil, fmt.Errorf("find payload %q: %w", info.PayloadMessage, err)
	}
	set := &descriptorpb.FileDescriptorSet{}
	seen := map[string]bool{}
	var add func(protoreflect.FileDescriptor)
	add = func(fd protoreflect.FileDescriptor) {
		if seen[fd.Path()] {
			return
		}
		seen[fd.Path()] = true
		imports := fd.Imports()
		for i := range imports.Len() {
			add(imports.Get(i).FileDescriptor)
		}
		set.File = append(set.File, protodesc.ToFileDescriptorProto(fd))
	}
	add(msg.Descriptor().ParentFile())
	return set, nil
}
