package protocol

import (
	"fmt"
	"time"
)

// NegotiatedCapabilities maps a capability ID to the revision both
// peers agreed on — always the highest common revision.
type NegotiatedCapabilities map[CapabilityID]uint16

// Negotiate computes the common capability set between two tables. It
// is symmetric: Negotiate(A, B) == Negotiate(B, A). Unknown capability
// IDs are ignored; capabilities without a common revision range are
// simply absent from the result. Whether the resulting set is
// sufficient is decided by Requirements, not by this function.
func Negotiate(peer, local AdvertisedCapabilities) NegotiatedCapabilities {
	out := make(NegotiatedCapabilities, min(len(peer), len(local)))
	for id, p := range peer {
		l, ok := local[id]
		if !ok {
			continue // unknown capability IDs are ignored, never rejected
		}
		lo := max(p.MinRevision, l.MinRevision)
		hi := min(p.MaxRevision, l.MaxRevision)
		if lo <= hi {
			out[id] = hi
		}
	}
	return out
}

// Requirements expresses the minimum revision a service needs for each
// capability. An empty Requirements map accepts any negotiated set.
type Requirements map[CapabilityID]uint16

// Check verifies every requirement against the negotiated set and
// returns a descriptive error for the first unsatisfied one.
func (r Requirements) Check(n NegotiatedCapabilities) error {
	for id, minRev := range r {
		rev, ok := n[id]
		if !ok {
			return fmt.Errorf("mvcp: capability 0x%02X not negotiated (required >= %d)", uint8(id), minRev)
		}
		if rev < minRev {
			return fmt.Errorf("mvcp: capability 0x%02X negotiated at revision %d, required >= %d", uint8(id), rev, minRev)
		}
	}
	return nil
}

// HandshakeTimeout bounds the entire prefix+HELLO exchange (both
// directions). It matches the existing vsock dial deadline convention
// (2s) and is cleared as soon as the handshake completes.
const HandshakeTimeout = 2 * time.Second
