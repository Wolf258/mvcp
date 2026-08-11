package protocol

import (
	"math/rand"
	"testing"
)

func TestNegotiateFullIntersection(t *testing.T) {
	peer := AdvertisedCapabilities{
		CapabilityExec:  {MinRevision: 1, MaxRevision: 2},
		CapabilityTools: {MinRevision: 1, MaxRevision: 1},
	}
	local := AdvertisedCapabilities{
		CapabilityExec:  {MinRevision: 1, MaxRevision: 3},
		CapabilityTools: {MinRevision: 2, MaxRevision: 2}, // no common range with peer 1..1
	}
	got := Negotiate(peer, local)
	want := NegotiatedCapabilities{CapabilityExec: 2}
	assertNegotiatedEqual(t, got, want)
}

func TestNegotiateEmptyAndUnknown(t *testing.T) {
	// Empty vs anything → empty.
	assertNegotiatedEqual(t, Negotiate(nil, DefaultCapabilities), NegotiatedCapabilities{})

	// Unknown capability IDs are ignored in both directions.
	peer := AdvertisedCapabilities{
		CapabilityID(0x7F): {MinRevision: 1, MaxRevision: 1}, // unknown to local
		CapabilityExec:     {MinRevision: 1, MaxRevision: 2},
	}
	local := AdvertisedCapabilities{
		CapabilityExec: {MinRevision: 1, MaxRevision: 2},
	}
	assertNegotiatedEqual(t, Negotiate(peer, local), NegotiatedCapabilities{CapabilityExec: 2})
	assertNegotiatedEqual(t, Negotiate(local, peer), NegotiatedCapabilities{CapabilityExec: 2})
}

func TestNegotiateEdgeCases(t *testing.T) {
	cases := []struct {
		name string
		peer AdvertisedCapabilities
		local AdvertisedCapabilities
		want NegotiatedCapabilities
	}{
		{
			name: "no common range",
			peer: AdvertisedCapabilities{CapabilityExec: {MinRevision: 3, MaxRevision: 4}},
			local: AdvertisedCapabilities{CapabilityExec: {MinRevision: 1, MaxRevision: 2}},
			want: NegotiatedCapabilities{},
		},
		{
			name: "exact single revision",
			peer: AdvertisedCapabilities{CapabilityExec: {MinRevision: 2, MaxRevision: 2}},
			local: AdvertisedCapabilities{CapabilityExec: {MinRevision: 1, MaxRevision: 3}},
			want: NegotiatedCapabilities{CapabilityExec: 2},
		},
		{
			name: "highest common revision is min of maxima",
			peer: AdvertisedCapabilities{CapabilityExec: {MinRevision: 1, MaxRevision: 2}},
			local: AdvertisedCapabilities{CapabilityExec: {MinRevision: 1, MaxRevision: 5}},
			want: NegotiatedCapabilities{CapabilityExec: 2},
		},
		{
			name: "peer narrower than local",
			peer: AdvertisedCapabilities{CapabilityExec: {MinRevision: 1, MaxRevision: 1}},
			local: AdvertisedCapabilities{CapabilityExec: {MinRevision: 1, MaxRevision: 2}},
			want: NegotiatedCapabilities{CapabilityExec: 1},
		},
		{
			name: "partial intersection",
			peer: AdvertisedCapabilities{
				CapabilityExec:  {MinRevision: 1, MaxRevision: 2},
				CapabilityTools: {MinRevision: 5, MaxRevision: 5},
			},
			local: AdvertisedCapabilities{
				CapabilityExec:  {MinRevision: 2, MaxRevision: 3},
				CapabilityTools: {MinRevision: 1, MaxRevision: 4},
			},
			want: NegotiatedCapabilities{CapabilityExec: 2},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertNegotiatedEqual(t, Negotiate(tc.peer, tc.local), tc.want)
		})
	}
}

// TestNegotiateSymmetryProperty checks Negotiate(A,B) == Negotiate(B,A)
// over many pseudo-random tables.
func TestNegotiateSymmetryProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	ids := []CapabilityID{CapabilityExec, CapabilityTools, CapabilityEvents, CapabilityFileTransfer, CapabilitySyncFS}
	for i := 0; i < 500; i++ {
		a := randomTable(rng, ids)
		b := randomTable(rng, ids)
		ab := Negotiate(a, b)
		ba := Negotiate(b, a)
		if !equalNegotiated(ab, ba) {
			t.Fatalf("Negotiate not symmetric:\nA=%v\nB=%v\nA,B=%v\nB,A=%v", a, b, ab, ba)
		}
	}
}

func randomTable(rng *rand.Rand, ids []CapabilityID) AdvertisedCapabilities {
	tbl := AdvertisedCapabilities{}
	for _, id := range ids {
		if rng.Intn(3) == 0 {
			continue // sometimes omit the capability entirely
		}
		minRev := uint16(rng.Intn(5) + 1)
		tbl[id] = CapabilitySupport{MinRevision: minRev, MaxRevision: minRev + uint16(rng.Intn(4))}
	}
	return tbl
}

func assertNegotiatedEqual(t *testing.T, got, want NegotiatedCapabilities) {
	t.Helper()
	if !equalNegotiated(got, want) {
		t.Fatalf("negotiated = %v, want %v", got, want)
	}
}

func equalNegotiated(a, b NegotiatedCapabilities) bool {
	if len(a) != len(b) {
		return false
	}
	for id, rev := range a {
		if b[id] != rev {
			return false
		}
	}
	return true
}
