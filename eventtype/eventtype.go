// Package eventtype lists the event types a Dingo node exposes through the
// EventService. The names are the exact strings the node's EventBus uses, so
// a type registered here is one the node emits.
package eventtype

import "slices"

// Peer governance events.
const (
	PeerChurn         = "peergov.peer_churn"
	BootstrapExited   = "peergov.bootstrap_exited"
	BootstrapRecovery = "peergov.bootstrap_recovery"
	QuotaStatus       = "peergov.quota_status"
	OutboundConn      = "peergov.outbound_conn"
)

// Chain events. A rollback is carried by ChainUpdate, not by a type of its
// own.
const (
	ChainUpdate       = "chain.update"
	ChainForkDetected = "chain.fork_detected"
)

// Ledger events.
const (
	LedgerError      = "ledger.error"
	LedgerBlockfetch = "ledger.blockfetch"
	LedgerChainsync  = "ledger.chainsync"
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
}

func info(name, category, description string) Info {
	return Info{Name: name, Category: category, Description: description}
}

var registered = []Info{
	info(PeerChurn, "peergov", "A peer was replaced or changed state in peer governance."),
	info(BootstrapExited, "peergov", "The node stopped using its bootstrap peers."),
	info(BootstrapRecovery, "peergov", "The node returned to its bootstrap peers."),
	info(QuotaStatus, "peergov", "Peer governance reported its peer quota status."),
	info(OutboundConn, "peergov", "An outbound connection to a peer was made."),
	info(ChainUpdate, "chain", "The chain tip changed, by a new block or a rollback."),
	info(ChainForkDetected, "chain", "A chain fork was detected."),
	info(LedgerError, "ledger", "The ledger reported an error."),
	info(LedgerBlockfetch, "ledger", "A blockfetch protocol event reached the ledger."),
	info(LedgerChainsync, "ledger", "A chainsync protocol event reached the ledger."),
	info(MempoolAddTx, "mempool", "A transaction was added to the mempool."),
	info(MempoolRemoveTx, "mempool", "A transaction was removed from the mempool."),
	info(ConnManagerInboundConn, "connmanager", "An inbound connection was accepted."),
	info(ConnManagerConnClosed, "connmanager", "A connection was closed."),
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
