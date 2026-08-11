package protocol

import (
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

// deadlineConn wraps a net.Conn and records deadlines. A non-zero
// deadline arms a timer that closes the underlying connection when it
// fires, so blocked reads/writes fail instead of hanging forever.
// net.Pipe itself does not support deadlines, hence the wrapper.
type deadlineConn struct {
	net.Conn
	mu       sync.Mutex
	deadline time.Time
	timer    *time.Timer
}

func newDeadlineConn(c net.Conn) *deadlineConn {
	return &deadlineConn{Conn: c}
}

func (c *deadlineConn) SetDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deadline = t
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	if !t.IsZero() {
		d := time.Until(t)
		if d < 0 {
			d = 0
		}
		c.timer = time.AfterFunc(d, func() { c.Close() })
	}
	return nil
}

func (c *deadlineConn) Deadline() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.deadline
}

type hsResult struct {
	peer       *Hello
	negotiated NegotiatedCapabilities
	err        error
}

func TestHandshakeHappyPath(t *testing.T) {
	serverEnd, clientEnd := net.Pipe()
	defer serverEnd.Close()
	defer clientEnd.Close()
	server := newDeadlineConn(serverEnd)
	client := newDeadlineConn(clientEnd)

	reqs := Requirements{CapabilityExec: 1, CapabilityTools: 1, CapabilitySyncFS: 1}

	serverCh := make(chan hsResult, 1)
	go func() {
		peer, negotiated, err := ServerHandshake(
			server,
			NewHello(RoleVHandler, "vhandler-0.8.2", DefaultCapabilities),
			[]PeerRole{RoleCore, RoleCLI},
			reqs,
		)
		serverCh <- hsResult{peer: peer, negotiated: negotiated, err: err}
	}()

	peer, negotiated, err := ClientHandshake(
		client,
		NewHello(RoleCore, "core-0.9.0", DefaultCapabilities),
		[]PeerRole{RoleVHandler},
		reqs,
	)
	if err != nil {
		t.Fatalf("client handshake: %v", err)
	}
	if peer.Role != RoleVHandler {
		t.Errorf("peer role = %d, want VHandler", peer.Role)
	}
	if peer.SoftwareVersion != "vhandler-0.8.2" {
		t.Errorf("peer version = %q", peer.SoftwareVersion)
	}
	if negotiated[CapabilityExec] != 2 || negotiated[CapabilityTools] != 1 || negotiated[CapabilitySyncFS] != 1 {
		t.Errorf("negotiated = %v", negotiated)
	}

	sr := waitHSResult(t, serverCh)
	if sr.err != nil {
		t.Fatalf("server handshake: %v", sr.err)
	}
	if sr.peer.Role != RoleCore {
		t.Errorf("server saw peer role %d, want Core", sr.peer.Role)
	}

	// The handshake deadline must be cleared before returning.
	if d := client.Deadline(); !d.IsZero() {
		t.Errorf("client deadline not cleared after handshake: %v", d)
	}
	if d := server.Deadline(); !d.IsZero() {
		t.Errorf("server deadline not cleared after handshake: %v", d)
	}
}

func TestHandshakeServerRejectsRole(t *testing.T) {
	serverEnd, clientEnd := net.Pipe()
	defer serverEnd.Close()
	defer clientEnd.Close()
	server := newDeadlineConn(serverEnd)
	client := newDeadlineConn(clientEnd)

	serverCh := make(chan hsResult, 1)
	go func() {
		_, _, err := ServerHandshake(
			server,
			NewHello(RoleVHandler, "v", DefaultCapabilities),
			[]PeerRole{RoleVHandler}, // only vhandlers allowed: the core client is rejected
			nil,
		)
		serverCh <- hsResult{err: err}
	}()

	if _, _, err := ClientHandshake(client, NewHello(RoleCore, "c", DefaultCapabilities), []PeerRole{RoleVHandler}, nil); err != nil {
		t.Fatalf("client handshake should not fail locally: %v", err)
	}

	// The server must have sent an ERROR frame with the same code. Read
	// it BEFORE waiting for the server goroutine: the server only
	// finishes once its ERROR write is consumed.
	frame, err := ReadMVCPFrame(client)
	if err != nil {
		t.Fatalf("read error frame: %v", err)
	}
	if frame.Type != TypeERROR || len(frame.Body) < 2 {
		t.Fatalf("expected ERROR frame, got type=0x%02X body=%x", frame.Type, frame.Body)
	}
	if code := binary.BigEndian.Uint16(frame.Body[:2]); code != ErrorCodeUnexpectedRole {
		t.Fatalf("error code = 0x%04X, want 0x%04X", code, ErrorCodeUnexpectedRole)
	}

	sr := waitHSResult(t, serverCh)
	var he *HandshakeError
	if !errors.As(sr.err, &he) || he.Code != ErrorCodeUnexpectedRole {
		t.Fatalf("server error = %v, want HandshakeError(UNEXPECTED_ROLE)", sr.err)
	}
}

func TestHandshakeClientRejectsRole(t *testing.T) {
	serverEnd, clientEnd := net.Pipe()
	defer serverEnd.Close()
	defer clientEnd.Close()
	server := newDeadlineConn(serverEnd)
	client := newDeadlineConn(clientEnd)

	serverCh := make(chan hsResult, 1)
	go func() {
		_, _, err := ServerHandshake(
			server,
			NewHello(RoleCLI, "v", DefaultCapabilities), // misbehaving peer: a CLI on the vhandler side
			[]PeerRole{RoleCore, RoleCLI},
			nil,
		)
		serverCh <- hsResult{err: err}
	}()

	_, _, err := ClientHandshake(client, NewHello(RoleCore, "c", DefaultCapabilities), []PeerRole{RoleVHandler}, nil)
	var he *HandshakeError
	if !errors.As(err, &he) || he.Code != ErrorCodeUnexpectedRole {
		t.Fatalf("client error = %v, want HandshakeError(UNEXPECTED_ROLE)", err)
	}
	// The server sees a closed connection (client rejected without replying).
	waitHSResult(t, serverCh)
}

func TestHandshakeRequirementsFailServerSide(t *testing.T) {
	serverEnd, clientEnd := net.Pipe()
	defer serverEnd.Close()
	defer clientEnd.Close()
	server := newDeadlineConn(serverEnd)
	client := newDeadlineConn(clientEnd)

	serverCh := make(chan hsResult, 1)
	go func() {
		_, _, err := ServerHandshake(
			server,
			NewHello(RoleVHandler, "v", DefaultCapabilities),
			[]PeerRole{RoleCore, RoleCLI},
			Requirements{CapabilityExec: 1},
		)
		serverCh <- hsResult{err: err}
	}()

	// Client advertises only Events → Exec requirement fails on the server.
	limited := AdvertisedCapabilities{CapabilityEvents: {MinRevision: 1, MaxRevision: 1}}
	if _, _, err := ClientHandshake(client, NewHello(RoleCore, "c", limited), []PeerRole{RoleVHandler}, nil); err != nil {
		t.Fatalf("client handshake should not fail locally: %v", err)
	}

	sr := waitHSResult(t, serverCh)
	var he *HandshakeError
	if !errors.As(sr.err, &he) || he.Code != ErrorCodeNoCommonCapability {
		t.Fatalf("server error = %v, want HandshakeError(NO_COMMON_CAPABILITY)", sr.err)
	}
}

func TestHandshakeRequirementsFailClientSide(t *testing.T) {
	serverEnd, clientEnd := net.Pipe()
	defer serverEnd.Close()
	defer clientEnd.Close()
	server := newDeadlineConn(serverEnd)
	client := newDeadlineConn(clientEnd)

	serverCh := make(chan hsResult, 1)
	go func() {
		_, _, err := ServerHandshake(
			server,
			NewHello(RoleVHandler, "v", AdvertisedCapabilities{CapabilityEvents: {MinRevision: 1, MaxRevision: 1}}),
			[]PeerRole{RoleCore, RoleCLI},
			nil,
		)
		serverCh <- hsResult{err: err}
	}()

	_, _, err := ClientHandshake(client, NewHello(RoleCore, "c", DefaultCapabilities), []PeerRole{RoleVHandler},
		Requirements{CapabilityExec: 1})
	var he *HandshakeError
	if !errors.As(err, &he) || he.Code != ErrorCodeNoCommonCapability {
		t.Fatalf("client error = %v, want HandshakeError(NO_COMMON_CAPABILITY)", err)
	}
	waitHSResult(t, serverCh)
}

// TestHandshakeMalformedPeerHello drives raw bytes on the client side:
// valid prefix + HELLO frame with a garbage body. The server must fail
// with errMalformedHello and send an ERROR(BAD_PAYLOAD) frame.
func TestHandshakeMalformedPeerHello(t *testing.T) {
	serverEnd, clientEnd := net.Pipe()
	defer serverEnd.Close()
	defer clientEnd.Close()
	server := newDeadlineConn(serverEnd)

	serverCh := make(chan hsResult, 1)
	go func() {
		_, _, err := ServerHandshake(server, NewHello(RoleVHandler, "v", DefaultCapabilities), []PeerRole{RoleCore, RoleCLI}, nil)
		serverCh <- hsResult{err: err}
	}()

	// Consume the server's prefix + HELLO.
	if err := validateMVCPPrefix(clientEnd); err != nil {
		t.Fatalf("read server prefix: %v", err)
	}
	frame, err := ReadMVCPFrame(clientEnd)
	if err != nil || frame.Type != TypeHELLO {
		t.Fatalf("read server hello: type=0x%02X err=%v", frame.Type, err)
	}
	// Send a valid prefix but a malformed HELLO body.
	if err := writeMVCPPrefix(clientEnd); err != nil {
		t.Fatal(err)
	}
	if err := WriteMVCPFrame(clientEnd, &Frame{Type: TypeHELLO, MsgID: 0, Body: []byte{0xFF, 0xFF, 0xFF}}); err != nil {
		t.Fatal(err)
	}

	errFrame, err := ReadMVCPFrame(clientEnd)
	if err != nil {
		t.Fatalf("read error frame: %v", err)
	}
	if errFrame.Type != TypeERROR || len(errFrame.Body) < 2 {
		t.Fatalf("expected ERROR frame, got type=0x%02X body=%x", errFrame.Type, errFrame.Body)
	}
	if code := binary.BigEndian.Uint16(errFrame.Body[:2]); code != ErrorCodeBadPayload {
		t.Fatalf("error code = 0x%04X, want 0x%04X (BAD_PAYLOAD)", code, ErrorCodeBadPayload)
	}

	sr := waitHSResult(t, serverCh)
	if !errors.Is(sr.err, errMalformedHello) {
		t.Fatalf("server error = %v, want errMalformedHello", sr.err)
	}
}

func TestHandshakeWireVersionMismatch(t *testing.T) {
	serverEnd, clientEnd := net.Pipe()
	defer serverEnd.Close()
	defer clientEnd.Close()
	server := newDeadlineConn(serverEnd)

	serverCh := make(chan hsResult, 1)
	go func() {
		_, _, err := ServerHandshake(server, NewHello(RoleVHandler, "v", DefaultCapabilities), []PeerRole{RoleCore, RoleCLI}, nil)
		serverCh <- hsResult{err: err}
	}()

	if err := validateMVCPPrefix(clientEnd); err != nil {
		t.Fatalf("read server prefix: %v", err)
	}
	if _, err := ReadMVCPFrame(clientEnd); err != nil {
		t.Fatalf("read server hello: %v", err)
	}
	// Bad wire version — the server must close WITHOUT an ERROR frame.
	if _, err := clientEnd.Write([]byte{'M', 'V', 'C', 'P', 0x02}); err != nil {
		t.Fatal(err)
	}

	sr := waitHSResult(t, serverCh)
	if !errors.Is(sr.err, ErrUnsupportedMVCPVersion) {
		t.Fatalf("server error = %v, want ErrUnsupportedMVCPVersion", sr.err)
	}
}

// TestHandshakeTimeout: a peer that never sends anything must produce a
// handshake error in about HandshakeTimeout, not hang forever.
func TestHandshakeTimeout(t *testing.T) {
	_, clientEnd := net.Pipe()
	defer clientEnd.Close()
	client := newDeadlineConn(clientEnd)

	start := time.Now()
	_, _, err := ClientHandshake(client, NewHello(RoleCore, "c", DefaultCapabilities), []PeerRole{RoleVHandler}, nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed > HandshakeTimeout+2*time.Second {
		t.Fatalf("handshake took %v, expected failure around %v", elapsed, HandshakeTimeout)
	}
	if elapsed < HandshakeTimeout-500*time.Millisecond {
		t.Fatalf("handshake failed too early (%v), deadline not enforced", elapsed)
	}
}

func waitHSResult(t *testing.T, ch chan hsResult) hsResult {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("handshake goroutine did not finish")
		return hsResult{}
	}
}
