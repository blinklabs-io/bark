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

var registered = []string{
	PeerChurn,
	BootstrapExited,
	BootstrapRecovery,
	QuotaStatus,
	OutboundConn,
	ChainUpdate,
	ChainForkDetected,
	LedgerError,
	LedgerBlockfetch,
	LedgerChainsync,
	MempoolAddTx,
	MempoolRemoveTx,
	ConnManagerInboundConn,
	ConnManagerConnClosed,
}

// Registered returns every registered event type. The caller may modify the
// returned slice.
func Registered() []string {
	return slices.Clone(registered)
}

// IsRegistered reports whether name is a registered event type.
func IsRegistered(name string) bool {
	return slices.Contains(registered, name)
}
