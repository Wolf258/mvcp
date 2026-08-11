package protocol

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"
)

// Wire prefix: "MVCP" magic + wire format version. The version byte is
// the single source of truth for the wire format; there is no version
// field inside HELLO (see docs/06-negotiation.md §2).
const (
	mvcpVersion    uint8 = 0x01
	mvcpMagicSize        = 4
)

var mvcpMagic = []byte{'M', 'V', 'C', 'P'}

// Wire-level handshake errors. These are returned locally and the
// connection is closed WITHOUT an ERROR frame: the peer may not be able
// to parse one.
var (
	ErrBadMVCPMagic          = errors.New("mvcp: bad handshake magic")
	ErrUnsupportedMVCPVersion = errors.New("mvcp: unsupported wire version")
)

// errHandshakeDeadlineRequired is returned when the connection does not
// support SetDeadline: a bounded handshake is mandatory (docs
// 06-negotiation.md §3).
var errHandshakeDeadlineRequired = errors.New("mvcp: handshake requires a connection with SetDeadline support")

type deadliner interface {
	SetDeadline(t time.Time) error
}

// HandshakeError describes a rejection sent to (or received from) the
// peer after the wire prefix was accepted, e.g. unexpected role or
// unsatisfied requirements.
type HandshakeError struct {
	Code    uint16
	Message string
}

func (e *HandshakeError) Error() string {
	return fmt.Sprintf("mvcp: handshake rejected: 0x%04X %s", e.Code, e.Message)
}

func writeMVCPPrefix(w io.Writer) error {
	prefix := [5]byte{'M', 'V', 'C', 'P', mvcpVersion}
	_, err := w.Write(prefix[:])
	return err
}

func validateMVCPPrefix(r io.Reader) error {
	var buf [5]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return err
	}
	if !bytes.Equal(buf[:mvcpMagicSize], mvcpMagic) {
		return ErrBadMVCPMagic
	}
	if buf[mvcpMagicSize] != mvcpVersion {
		return ErrUnsupportedMVCPVersion
	}
	return nil
}

// writeHandshakeError sends an ERROR frame (msg_id=0, IS_RESPONSE).
// Only safe after both peers have accepted the wire prefix.
func writeHandshakeError(w io.Writer, code uint16, message string) error {
	var buf bytes.Buffer
	if err := WriteUint16(&buf, code); err != nil {
		return err
	}
	if err := WriteString(&buf, message); err != nil {
		return err
	}
	return WriteMVCPFrame(w, &Frame{Type: TypeERROR, Flags: FlagResponse, MsgID: 0, Body: buf.Bytes()})
}

// withHandshakeDeadline arms the handshake deadline on rw and returns a
// clear function that restores an unbounded deadline. The clear MUST be
// called (deferred) so the handshake deadline never contaminates normal
// connection traffic.
func withHandshakeDeadline(rw io.ReadWriter) (func(), error) {
	dl, ok := rw.(deadliner)
	if !ok {
		return nil, errHandshakeDeadlineRequired
	}
	if err := dl.SetDeadline(time.Now().Add(HandshakeTimeout)); err != nil {
		return nil, fmt.Errorf("mvcp: set handshake deadline: %w", err)
	}
	return func() { dl.SetDeadline(time.Time{}) }, nil
}

// readPeerHello reads and validates the peer's wire prefix and HELLO
// frame. Wire-level failures (I/O, bad magic, bad version) are returned
// as-is; a malformed HELLO is wrapped with errMalformedHello so callers
// can decide whether an ERROR frame is safe to send.
func readPeerHello(r io.Reader) (*Hello, error) {
	if err := validateMVCPPrefix(r); err != nil {
		return nil, err
	}
	frame, err := ReadMVCPFrame(r)
	if err != nil {
		return nil, err
	}
	if frame.Type != TypeHELLO {
		return nil, fmt.Errorf("%w: expected HELLO, got 0x%02X", errMalformedHello, frame.Type)
	}
	h := &Hello{}
	if err := h.UnmarshalBinary(frame.Body); err != nil {
		return nil, err
	}
	return h, nil
}

func roleAccepted(role PeerRole, roles []PeerRole) bool {
	for _, r := range roles {
		if role == r {
			return true
		}
	}
	return false
}

// ServerHandshake performs the MVCP handshake on the accepting side
// (vhandler sessions). It writes the wire prefix + its HELLO first,
// reads and validates the peer's prefix + HELLO, then negotiates and
// enforces reqs. On rejection the peer is sent an ERROR frame (when the
// wire prefix has been accepted) and a *HandshakeError is returned; the
// caller must close the connection. The handshake deadline is applied
// for the whole exchange and cleared before returning.
func ServerHandshake(rw io.ReadWriter, local *Hello, acceptRoles []PeerRole, reqs Requirements) (*Hello, NegotiatedCapabilities, error) {
	clear, err := withHandshakeDeadline(rw)
	if err != nil {
		return nil, nil, err
	}
	defer clear()

	if err := writeMVCPPrefix(rw); err != nil {
		return nil, nil, fmt.Errorf("mvcp: server handshake: write prefix: %w", err)
	}
	body, err := local.MarshalBinary()
	if err != nil {
		return nil, nil, fmt.Errorf("mvcp: server handshake: encode hello: %w", err)
	}
	if err := WriteMVCPFrame(rw, &Frame{Type: TypeHELLO, MsgID: 0, Body: body}); err != nil {
		return nil, nil, fmt.Errorf("mvcp: server handshake: write hello: %w", err)
	}

	peer, err := readPeerHello(rw)
	if err != nil {
		if errors.Is(err, errMalformedHello) {
			writeHandshakeError(rw, ErrorCodeBadPayload, err.Error())
		}
		return nil, nil, fmt.Errorf("mvcp: server handshake: read peer hello: %w", err)
	}
	if !roleAccepted(peer.Role, acceptRoles) {
		msg := fmt.Sprintf("unexpected peer role %d", peer.Role)
		writeHandshakeError(rw, ErrorCodeUnexpectedRole, msg)
		return nil, nil, &HandshakeError{Code: ErrorCodeUnexpectedRole, Message: msg}
	}
	negotiated := Negotiate(peer.advertised(), local.advertised())
	if err := reqs.Check(negotiated); err != nil {
		writeHandshakeError(rw, ErrorCodeNoCommonCapability, err.Error())
		return nil, nil, &HandshakeError{Code: ErrorCodeNoCommonCapability, Message: err.Error()}
	}
	return peer, negotiated, nil
}

// ClientHandshake performs the MVCP handshake on the dialing side
// (shifty-core, shiftyctl). It reads and validates the peer's prefix +
// HELLO first, then writes its own prefix + HELLO, negotiates and
// enforces reqs. On rejection the caller must close the connection; the
// peer observes a closed connection on its next I/O. The handshake
// deadline is applied for the whole exchange and cleared before
// returning.
func ClientHandshake(rw io.ReadWriter, local *Hello, expectRoles []PeerRole, reqs Requirements) (*Hello, NegotiatedCapabilities, error) {
	clear, err := withHandshakeDeadline(rw)
	if err != nil {
		return nil, nil, err
	}
	defer clear()

	peer, err := readPeerHello(rw)
	if err != nil {
		// The client has not written its own prefix yet, so an ERROR
		// frame here would be wire-invalid (the peer expects a prefix).
		// Close without replying; the peer observes the closed
		// connection.
		return nil, nil, fmt.Errorf("mvcp: client handshake: read peer hello: %w", err)
	}
	if !roleAccepted(peer.Role, expectRoles) {
		return nil, nil, &HandshakeError{Code: ErrorCodeUnexpectedRole, Message: fmt.Sprintf("unexpected peer role %d", peer.Role)}
	}
	if err := writeMVCPPrefix(rw); err != nil {
		return nil, nil, fmt.Errorf("mvcp: client handshake: write prefix: %w", err)
	}
	body, err := local.MarshalBinary()
	if err != nil {
		return nil, nil, fmt.Errorf("mvcp: client handshake: encode hello: %w", err)
	}
	if err := WriteMVCPFrame(rw, &Frame{Type: TypeHELLO, MsgID: 0, Body: body}); err != nil {
		return nil, nil, fmt.Errorf("mvcp: client handshake: write hello: %w", err)
	}

	negotiated := Negotiate(peer.advertised(), local.advertised())
	if err := reqs.Check(negotiated); err != nil {
		writeHandshakeError(rw, ErrorCodeNoCommonCapability, err.Error())
		return nil, nil, &HandshakeError{Code: ErrorCodeNoCommonCapability, Message: err.Error()}
	}
	return peer, negotiated, nil
}
