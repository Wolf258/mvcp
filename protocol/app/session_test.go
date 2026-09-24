package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/Wolf258/mvcp/protocol"
	"github.com/Wolf258/mvcp/protocol/messages"
)

type handlerFunc func(ctx context.Context, service string, meta []byte) (Endpoint, []byte, error)

func (f handlerFunc) Open(ctx context.Context, service string, meta []byte) (Endpoint, []byte, error) {
	return f(ctx, service, meta)
}

// pipeEndpoint is an in-memory Endpoint plus a client handle to drive it.
type pipeEndpoint struct {
	r *io.PipeReader
	w *io.PipeWriter
}

func (e *pipeEndpoint) Read(p []byte) (int, error)  { return e.r.Read(p) }
func (e *pipeEndpoint) Write(p []byte) (int, error) { return e.w.Write(p) }
func (e *pipeEndpoint) Close() error                { e.r.Close(); e.w.Close(); return nil }

// CloseWrite half-closes the endpoint (net.Conn semantics): the peer
// sees EOF, but the endpoint can still read replies.
func (e *pipeEndpoint) CloseWrite() error { return e.w.Close() }

type endpointClient struct {
	io.Reader
	io.Writer
	r *io.PipeReader
	w *io.PipeWriter
}

func (c *endpointClient) Close() error { c.r.Close(); c.w.Close(); return nil }

func newPipeEndpoint() (*pipeEndpoint, *endpointClient) {
	epR, cW := io.Pipe()
	cR, epW := io.Pipe()
	return &pipeEndpoint{r: epR, w: epW}, &endpointClient{Reader: cR, Writer: cW, r: cR, w: cW}
}

func newPair(t *testing.T, hostH, guestH Handler) (*Session, *Session) {
	t.Helper()
	c1, c2 := net.Pipe()
	host := NewSession(c1, Config{Role: RoleHost, Handler: hostH})
	guest := NewSession(c2, Config{Role: RoleGuest, Handler: guestH})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = host.Serve(ctx) }()
	go func() { _ = guest.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		host.Close()
		guest.Close()
	})
	return host, guest
}

type observerFunc func(*Stream)

func (f observerFunc) StreamOpened(*Stream) {}

func (f observerFunc) StreamClosed(st *Stream) { f(st) }

func TestOpenAcceptEcho(t *testing.T) {
	_, guest := newPair(t,
		handlerFunc(func(ctx context.Context, service string, meta []byte) (Endpoint, []byte, error) {
			if service != "echo" || string(meta) != "hi" {
				return nil, nil, &RejectError{Code: protocol.ErrorCodeAppServiceNotFound, Message: "unknown"}
			}
			ep, client := newPipeEndpoint()
			go func() { _, _ = io.Copy(client, client) }() // echo per chunk
			return ep, []byte("accepted"), nil
		}), handlerFunc(func(context.Context, string, []byte) (Endpoint, []byte, error) {
			return nil, nil, errors.New("guest handler must not be used")
		}))
	st, err := guest.Open(context.Background(), "echo", []byte("hi"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if string(st.AcceptMeta()) != "accepted" {
		t.Fatalf("accept meta = %q", st.AcceptMeta())
	}
	if _, err := st.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(st, buf); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if !bytes.Equal(buf, []byte("ping")) {
		t.Fatalf("echo = %q, want ping", buf)
	}
}

func TestOpenRejected(t *testing.T) {
	host, guest := newPair(t, handlerFunc(func(context.Context, string, []byte) (Endpoint, []byte, error) {
		return nil, nil, &RejectError{Code: protocol.ErrorCodeAppNotAuthorized, Message: "denied"}
	}), handlerFunc(func(context.Context, string, []byte) (Endpoint, []byte, error) {
		return nil, nil, errors.New("unused")
	}))
	_, err := guest.Open(context.Background(), "svc", nil)
	var rej *RejectError
	if !errors.As(err, &rej) || rej.Code != protocol.ErrorCodeAppNotAuthorized {
		t.Fatalf("Open err = %v, want RejectError NOT_AUTHORIZED", err)
	}
	_ = host
}

func TestUnknownStreamFramesIgnored(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	session := NewSession(serverConn, Config{Role: RoleHost, Handler: handlerFunc(
		func(context.Context, string, []byte) (Endpoint, []byte, error) {
			ep, _ := newPipeEndpoint()
			return ep, nil, nil
		})})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = session.Serve(ctx) }()
	defer session.Close()

	unknown := uint32(0x800000FF)
	frames := []*protocol.Frame{
		{Type: protocol.TypeAPPDATA, Body: mustMarshal(t, &messages.AppData{StreamID: unknown, Data: []byte("x")})},
		{Type: protocol.TypeAPPCREDIT, Body: mustMarshal(t, &messages.AppCredit{StreamID: unknown, Bytes: 10})},
		{Type: protocol.TypeAPPCLOSE, Body: mustMarshal(t, &messages.AppClose{StreamID: unknown})},
		{Type: protocol.TypeAPPRESET, Body: mustMarshal(t, &messages.AppReset{StreamID: unknown, Code: protocol.ErrorCodeAppLocalError})},
	}
	for _, f := range frames {
		if err := protocol.WriteMVCPFrame(clientConn, f); err != nil {
			t.Fatalf("write unknown frame: %v", err)
		}
	}
	// Connection must still be alive: a valid guest-initiated APP_OPEN
	// (bit31=1) gets an ACCEPT.
	open := mustMarshal(t, &messages.AppOpen{StreamID: 0x80000001, Service: "svc"})
	if err := protocol.WriteMVCPFrame(clientConn, &protocol.Frame{Type: protocol.TypeAPPOPEN, Body: open}); err != nil {
		t.Fatalf("write open: %v", err)
	}
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	frame, err := protocol.ReadMVCPFrame(clientConn)
	if err != nil {
		t.Fatalf("read accept: %v", err)
	}
	if frame.Type != protocol.TypeAPPACCEPT {
		t.Fatalf("frame type = 0x%02X, want ACCEPT", frame.Type)
	}
}

func TestMalformedBodyClosesConnection(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	session := NewSession(serverConn, Config{Role: RoleHost, Handler: handlerFunc(
		func(context.Context, string, []byte) (Endpoint, []byte, error) { return nil, nil, nil })})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = session.Serve(ctx) }()
	defer session.Close()

	if err := protocol.WriteMVCPFrame(clientConn, &protocol.Frame{Type: protocol.TypeAPPOPEN, Body: []byte("trash")}); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	frame, err := protocol.ReadMVCPFrame(clientConn)
	if err != nil {
		t.Fatalf("expected ERROR frame, got %v", err)
	}
	if frame.Type != protocol.TypeERROR {
		t.Fatalf("frame type = 0x%02X, want ERROR", frame.Type)
	}
	code, _ := protocol.ReadUint16(bytes.NewReader(frame.Body))
	if code != protocol.ErrorCodeBadPayload {
		t.Fatalf("error code = 0x%04X, want BAD_PAYLOAD", code)
	}
	if _, err := protocol.ReadMVCPFrame(clientConn); err == nil {
		t.Fatal("connection still open after malformed body")
	}
}

func TestStreamIDsDoNotCollide(t *testing.T) {
	host, guest := newPair(t,
		handlerFunc(func(context.Context, string, []byte) (Endpoint, []byte, error) {
			ep, _ := newPipeEndpoint()
			return ep, nil, nil
		}),
		handlerFunc(func(context.Context, string, []byte) (Endpoint, []byte, error) {
			ep, _ := newPipeEndpoint()
			return ep, nil, nil
		}))
	hs, err := host.Open(context.Background(), "svc", nil)
	if err != nil {
		t.Fatalf("host open: %v", err)
	}
	gs, err := guest.Open(context.Background(), "svc", nil)
	if err != nil {
		t.Fatalf("guest open: %v", err)
	}
	if hs.ID()&0x80000000 != 0 {
		t.Fatalf("host stream id 0x%08X has bit31 set", hs.ID())
	}
	if gs.ID()&0x80000000 == 0 {
		t.Fatalf("guest stream id 0x%08X has bit31 clear", gs.ID())
	}
	if _, err := hs.Write([]byte("h")); err != nil {
		t.Fatalf("host write: %v", err)
	}
	if _, err := gs.Write([]byte("g")); err != nil {
		t.Fatalf("guest write: %v", err)
	}
}

func TestMaxStreamsRejected(t *testing.T) {
	host, guest := newPair(t, handlerFunc(func(context.Context, string, []byte) (Endpoint, []byte, error) {
		ep, _ := newPipeEndpoint()
		return ep, nil, nil
	}), handlerFunc(func(context.Context, string, []byte) (Endpoint, []byte, error) {
		return nil, nil, errors.New("unused")
	}))
	_ = host
	for i := 0; i < protocol.AppMaxStreams; i++ {
		if _, err := guest.Open(context.Background(), "svc", nil); err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
	}
	_, err := guest.Open(context.Background(), "svc", nil)
	var rej *RejectError
	if !errors.As(err, &rej) || rej.Code != protocol.ErrorCodeAppQuotaExceeded {
		t.Fatalf("65th open err = %v, want QUOTA_EXCEEDED", err)
	}
}

func TestOpenFailsWhenStreamIDSpaceExhausted(t *testing.T) {
	// Stream IDs are never reused within a connection (app-channel.md),
	// so an exhausted 31-bit local space must fail the open instead of
	// handing out an ID that collides with the peer's range.
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	go func() { _, _ = io.Copy(io.Discard, c1) }() // drain APP_OPEN/APP_RESET if sent
	s := NewSession(c1, Config{Role: RoleHost})
	s.mu.Lock()
	s.nextID = maxLocalStreamID
	s.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Open must fail before it starts waiting for APP_ACCEPT
	_, err := s.Open(ctx, "svc", nil)
	if !errors.Is(err, ErrStreamIDExhausted) {
		t.Fatalf("Open err = %v, want ErrStreamIDExhausted", err)
	}
}

func mustMarshal(t *testing.T, m interface{ MarshalBinary() ([]byte, error) }) []byte {
	t.Helper()
	body, err := m.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return body
}
