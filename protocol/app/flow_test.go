package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/Wolf258/mvcp/protocol"
	"github.com/Wolf258/mvcp/protocol/messages"
)

func TestSendQueueOverflowResets(t *testing.T) {
	// The host handler accepts but never reads the endpoint: the guest
	// cannot get credit back, queues past 1 MiB and must observe a reset
	// instead of silent loss.
	host, guest := newPair(t, handlerFunc(func(context.Context, string, []byte) (Endpoint, []byte, error) {
		ep, _ := newPipeEndpoint()
		return ep, nil, nil
	}), handlerFunc(func(context.Context, string, []byte) (Endpoint, []byte, error) {
		return nil, nil, errors.New("unused")
	}))
	_ = host
	st, err := guest.Open(context.Background(), "svc", nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	chunk := make([]byte, 64<<10)
	var writeErr error
	for i := 0; i < 32; i++ { // 2 MiB > 1 MiB queue + 256 KiB credit
		if _, writeErr = st.Write(chunk); writeErr != nil {
			break
		}
	}
	var reset *ResetError
	if !errors.As(writeErr, &reset) || reset.Code != protocol.ErrorCodeAppOverflow {
		t.Fatalf("write err = %v, want OVERFLOW reset", writeErr)
	}
}

func TestCreditCapResets(t *testing.T) {
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

	if err := protocol.WriteMVCPFrame(clientConn, &protocol.Frame{
		Type: protocol.TypeAPPOPEN,
		Body: mustMarshal(t, &messages.AppOpen{StreamID: 0x80000001, Service: "svc"}),
	}); err != nil {
		t.Fatalf("open: %v", err)
	}
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := protocol.ReadMVCPFrame(clientConn); err != nil { // ACCEPT
		t.Fatalf("accept: %v", err)
	}
	// Over-grant credit for the stream: outstanding would exceed 4x window.
	body := mustMarshal(t, &messages.AppCredit{StreamID: 0x80000001, Bytes: protocol.AppMaxCredit + 1})
	if err := protocol.WriteMVCPFrame(clientConn, &protocol.Frame{Type: protocol.TypeAPPCREDIT, Body: body}); err != nil {
		t.Fatalf("credit: %v", err)
	}
	frame, err := protocol.ReadMVCPFrame(clientConn)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if frame.Type != protocol.TypeAPPRESET {
		t.Fatalf("frame type = 0x%02X, want APP_RESET", frame.Type)
	}
	var rst messages.AppReset
	if err := rst.UnmarshalBinary(frame.Body); err != nil {
		t.Fatalf("decode reset: %v", err)
	}
	if rst.Code != protocol.ErrorCodeAppProtocolError {
		t.Fatalf("reset code = 0x%04X, want PROTOCOL_ERROR", rst.Code)
	}
}

func TestCreditCapOnAcceptGrant(t *testing.T) {
	// The session under test is the opener: it validates the grant the
	// peer sends in APP_ACCEPT and resets the stream above AppMaxCredit.
	serverConn, clientConn := net.Pipe()
	session := NewSession(serverConn, Config{Role: RoleHost, Handler: handlerFunc(
		func(context.Context, string, []byte) (Endpoint, []byte, error) {
			return nil, nil, errors.New("no inbound streams in this test")
		})})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = session.Serve(ctx) }()
	defer session.Close()

	openErr := make(chan error, 1)
	go func() {
		_, err := session.Open(ctx, "svc", nil)
		openErr <- err
	}()

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	frame, err := protocol.ReadMVCPFrame(clientConn)
	if err != nil {
		t.Fatalf("read APP_OPEN: %v", err)
	}
	var open messages.AppOpen
	if err := open.UnmarshalBinary(frame.Body); err != nil {
		t.Fatalf("decode open: %v", err)
	}
	if open.StreamID&0x80000000 != 0 {
		t.Fatalf("host-initiated stream id 0x%08X has bit31 set", open.StreamID)
	}
	body := mustMarshal(t, &messages.AppAccept{StreamID: open.StreamID, Grant: protocol.AppMaxCredit + 1})
	if err := protocol.WriteMVCPFrame(clientConn, &protocol.Frame{Type: protocol.TypeAPPACCEPT, Body: body}); err != nil {
		t.Fatalf("write accept: %v", err)
	}
	frame, err = protocol.ReadMVCPFrame(clientConn)
	if err != nil {
		t.Fatalf("read APP_RESET: %v", err)
	}
	if frame.Type != protocol.TypeAPPRESET {
		t.Fatalf("frame type = 0x%02X, want APP_RESET", frame.Type)
	}
	var rst messages.AppReset
	if err := rst.UnmarshalBinary(frame.Body); err != nil {
		t.Fatalf("decode reset: %v", err)
	}
	if rst.Code != protocol.ErrorCodeAppProtocolError {
		t.Fatalf("reset code = 0x%04X, want PROTOCOL_ERROR", rst.Code)
	}
	select {
	case err := <-openErr:
		var reset *ResetError
		if !errors.As(err, &reset) {
			t.Fatalf("Open err = %v, want ResetError", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Open did not return after inflated ACCEPT grant")
	}
}

func TestSessionBufferCapUnit(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	s := NewSession(c1, Config{Role: RoleHost})
	if !s.reserveBuffer(protocol.AppMaxBuffered) {
		t.Fatal("reserveBuffer(max) = false, want true")
	}
	if s.reserveBuffer(1) {
		t.Fatal("reserveBuffer over cap = true, want false")
	}
	s.releaseBuffer(protocol.AppMaxBuffered)
	if !s.reserveBuffer(1) {
		t.Fatal("reserveBuffer after release = false, want true")
	}
}

func TestSlowConsumerDoesNotLoseData(t *testing.T) {
	// Host endpoint reads slowly; guest writes 300 KiB. Credits must
	// replenish and every byte must arrive.
	got := make(chan []byte, 1)
	host, guest := newPair(t, handlerFunc(func(context.Context, string, []byte) (Endpoint, []byte, error) {
		ep, client := newPipeEndpoint()
		go func() {
			var buf bytes.Buffer
			tmp := make([]byte, 8<<10)
			for {
				n, err := client.Read(tmp)
				if n > 0 {
					buf.Write(tmp[:n])
					time.Sleep(time.Millisecond) // slow consumer
				}
				if err != nil {
					got <- buf.Bytes()
					return
				}
			}
		}()
		return ep, nil, nil
	}), handlerFunc(func(context.Context, string, []byte) (Endpoint, []byte, error) {
		return nil, nil, errors.New("unused")
	}))
	_ = host
	st, err := guest.Open(context.Background(), "svc", nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	want := bytes.Repeat([]byte{0xAB}, 300<<10)
	done := make(chan error, 1)
	go func() {
		_, err := st.Write(want)
		done <- err
	}()
	if err := <-done; err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = st.CloseWrite()
	select {
	case data := <-got:
		if !bytes.Equal(data, want) {
			t.Fatalf("received %d bytes, want %d", len(data), len(want))
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for data")
	}
}

func TestCloseResetsStreamsWithPeerGone(t *testing.T) {
	c1, c2 := net.Pipe()
	closed := make(chan *Stream, 1)
	host := NewSession(c1, Config{
		Role: RoleHost,
		Handler: handlerFunc(func(context.Context, string, []byte) (Endpoint, []byte, error) {
			ep, _ := newPipeEndpoint()
			return ep, nil, nil
		}),
		Observer: observerFunc(func(st *Stream) { closed <- st }),
	})
	guest := NewSession(c2, Config{Role: RoleGuest, Handler: handlerFunc(
		func(context.Context, string, []byte) (Endpoint, []byte, error) {
			return nil, nil, errors.New("unused")
		})})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = host.Serve(ctx) }()
	go func() { _ = guest.Serve(ctx) }()
	t.Cleanup(func() { host.Close(); guest.Close() })

	if _, err := guest.Open(context.Background(), "svc", nil); err != nil {
		t.Fatalf("open: %v", err)
	}
	_ = guest.Close() // dropped connection -> host streams reset PEER_GONE
	select {
	case st := <-closed:
		var reset *ResetError
		if err := st.terminalError(); !errors.As(err, &reset) || reset.Code != protocol.ErrorCodeAppPeerGone {
			t.Fatalf("terminalError = %v, want PEER_GONE", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("observer did not report stream close")
	}
}

func TestHalfCloseDeliversEOFThenPeerCanReply(t *testing.T) {
	hostGot := make(chan []byte, 1)
	host, guest := newPair(t, handlerFunc(func(context.Context, string, []byte) (Endpoint, []byte, error) {
		ep, client := newPipeEndpoint()
		go func() {
			data, _ := io.ReadAll(client) // EOF after guest CloseWrite
			hostGot <- data
			_, _ = client.Write([]byte("reply"))
		}()
		return ep, nil, nil
	}), handlerFunc(func(context.Context, string, []byte) (Endpoint, []byte, error) {
		return nil, nil, errors.New("unused")
	}))
	_ = host
	st, err := guest.Open(context.Background(), "svc", nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := st.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := st.CloseWrite(); err != nil {
		t.Fatalf("close write: %v", err)
	}
	select {
	case data := <-hostGot:
		if string(data) != "hello" {
			t.Fatalf("host got %q, want hello", data)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("host did not see EOF after APP_CLOSE")
	}
	buf := make([]byte, 5)
	if _, err := io.ReadFull(st, buf); err != nil {
		t.Fatalf("read reply after half-close: %v", err)
	}
	if string(buf) != "reply" {
		t.Fatalf("reply = %q", buf)
	}
}

// encodeOpen builds an APP_OPEN body by hand so a test can carry an
// oversized meta that MarshalBinary would refuse.
func encodeOpen(t *testing.T, streamID uint32, service string, meta []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	protocol.WriteUint32(&buf, streamID)
	protocol.WriteString(&buf, service)
	protocol.WriteBytes(&buf, meta)
	return buf.Bytes()
}

func TestInvalidAppOpenRejected(t *testing.T) {
	cases := map[string][]byte{
		"bad service":    encodeOpen(t, 0x80000001, "Bad", nil),
		"oversized meta": encodeOpen(t, 0x80000002, "svc", make([]byte, protocol.AppMaxMetaBytes+1)),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			serverConn, clientConn := net.Pipe()
			session := NewSession(serverConn, Config{Role: RoleHost, Handler: handlerFunc(
				func(context.Context, string, []byte) (Endpoint, []byte, error) {
					t.Error("handler must not run for an invalid APP_OPEN")
					return nil, nil, errors.New("unused")
				})})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() { _ = session.Serve(ctx) }()
			defer session.Close()

			if err := protocol.WriteMVCPFrame(clientConn, &protocol.Frame{Type: protocol.TypeAPPOPEN, Body: body}); err != nil {
				t.Fatalf("write: %v", err)
			}
			_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
			frame, err := protocol.ReadMVCPFrame(clientConn)
			if err != nil {
				t.Fatalf("read reject: %v", err)
			}
			if frame.Type != protocol.TypeAPPREJECT {
				t.Fatalf("frame type = 0x%02X, want APP_REJECT", frame.Type)
			}
			var rej messages.AppReject
			if err := rej.UnmarshalBinary(frame.Body); err != nil {
				t.Fatalf("decode reject: %v", err)
			}
			if rej.Code != protocol.ErrorCodeAppProtocolError {
				t.Fatalf("reject code = 0x%04X, want PROTOCOL_ERROR", rej.Code)
			}
		})
	}
}

func TestOversizedAcceptMetaResets(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	session := NewSession(serverConn, Config{Role: RoleHost, Handler: handlerFunc(
		func(context.Context, string, []byte) (Endpoint, []byte, error) {
			return nil, nil, errors.New("no inbound streams in this test")
		})})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = session.Serve(ctx) }()
	defer session.Close()

	openErr := make(chan error, 1)
	go func() {
		_, err := session.Open(ctx, "svc", nil)
		openErr <- err
	}()

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	frame, err := protocol.ReadMVCPFrame(clientConn)
	if err != nil {
		t.Fatalf("read APP_OPEN: %v", err)
	}
	var open messages.AppOpen
	if err := open.UnmarshalBinary(frame.Body); err != nil {
		t.Fatalf("decode open: %v", err)
	}
	var buf bytes.Buffer
	protocol.WriteUint32(&buf, open.StreamID)
	protocol.WriteBytes(&buf, make([]byte, protocol.AppMaxMetaBytes+1))
	protocol.WriteUint32(&buf, protocol.AppInitialWindow)
	if err := protocol.WriteMVCPFrame(clientConn, &protocol.Frame{Type: protocol.TypeAPPACCEPT, Body: buf.Bytes()}); err != nil {
		t.Fatalf("write accept: %v", err)
	}
	frame, err = protocol.ReadMVCPFrame(clientConn)
	if err != nil {
		t.Fatalf("read APP_RESET: %v", err)
	}
	if frame.Type != protocol.TypeAPPRESET {
		t.Fatalf("frame type = 0x%02X, want APP_RESET", frame.Type)
	}
	var rst messages.AppReset
	if err := rst.UnmarshalBinary(frame.Body); err != nil {
		t.Fatalf("decode reset: %v", err)
	}
	if rst.Code != protocol.ErrorCodeAppProtocolError {
		t.Fatalf("reset code = 0x%04X, want PROTOCOL_ERROR", rst.Code)
	}
	select {
	case err := <-openErr:
		var reset *ResetError
		if !errors.As(err, &reset) {
			t.Fatalf("Open err = %v, want ResetError", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Open did not return after oversized ACCEPT meta")
	}
}

func TestOversizedDataResets(t *testing.T) {
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

	if err := protocol.WriteMVCPFrame(clientConn, &protocol.Frame{
		Type: protocol.TypeAPPOPEN,
		Body: mustMarshal(t, &messages.AppOpen{StreamID: 0x80000001, Service: "svc"}),
	}); err != nil {
		t.Fatalf("open: %v", err)
	}
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := protocol.ReadMVCPFrame(clientConn); err != nil { // ACCEPT
		t.Fatalf("accept: %v", err)
	}
	var buf bytes.Buffer
	protocol.WriteUint32(&buf, 0x80000001)
	protocol.WriteBytes(&buf, make([]byte, protocol.AppMaxDataBytes+1))
	if err := protocol.WriteMVCPFrame(clientConn, &protocol.Frame{Type: protocol.TypeAPPDATA, Body: buf.Bytes()}); err != nil {
		t.Fatalf("data: %v", err)
	}
	frame, err := protocol.ReadMVCPFrame(clientConn)
	if err != nil {
		t.Fatalf("read reset: %v", err)
	}
	if frame.Type != protocol.TypeAPPRESET {
		t.Fatalf("frame type = 0x%02X, want APP_RESET", frame.Type)
	}
	var rst messages.AppReset
	if err := rst.UnmarshalBinary(frame.Body); err != nil {
		t.Fatalf("decode reset: %v", err)
	}
	if rst.Code != protocol.ErrorCodeAppProtocolError {
		t.Fatalf("reset code = 0x%04X, want PROTOCOL_ERROR", rst.Code)
	}
}

// gatedEndpoint is an acceptor endpoint whose writes stay blocked until
// the test releases the gate. It lets a test hold the endpoint stalled
// while both sides half-close, with inbound data still buffered.
type gatedEndpoint struct {
	gate    chan struct{}
	done    chan struct{}
	gateOne sync.Once
	doneOne sync.Once

	mu     sync.Mutex
	buf    bytes.Buffer
	closed bool
}

func newGatedEndpoint() *gatedEndpoint {
	return &gatedEndpoint{gate: make(chan struct{}), done: make(chan struct{})}
}

func (g *gatedEndpoint) Read([]byte) (int, error) { return 0, io.EOF }

func (g *gatedEndpoint) Write(p []byte) (int, error) {
	select {
	case <-g.done:
		return 0, io.ErrClosedPipe
	case <-g.gate:
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return 0, io.ErrClosedPipe
	}
	return g.buf.Write(p)
}

// CloseWrite is a no-op: this endpoint only buffers writes.
func (g *gatedEndpoint) CloseWrite() error { return nil }

func (g *gatedEndpoint) Close() error {
	g.mu.Lock()
	g.closed = true
	g.mu.Unlock()
	g.doneOne.Do(func() { close(g.done) })
	return nil
}

func (g *gatedEndpoint) release() { g.gateOne.Do(func() { close(g.gate) }) }

func (g *gatedEndpoint) data() []byte {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]byte(nil), g.buf.Bytes()...)
}

func TestCleanCloseKeepsBufferedData(t *testing.T) {
	// The acceptor half-closes first and only drains afterwards; every
	// byte the opener sent before its APP_CLOSE must still reach the
	// acceptor endpoint (spec §4.9: a half-closed peer keeps receiving).
	closed := make(chan *Stream, 1)
	serverConn, clientConn := net.Pipe()
	eps := make(chan *gatedEndpoint, 1)
	host := NewSession(serverConn, Config{
		Role: RoleHost,
		Handler: handlerFunc(func(context.Context, string, []byte) (Endpoint, []byte, error) {
			ep := newGatedEndpoint()
			eps <- ep
			return ep, nil, nil
		}),
		Observer: observerFunc(func(st *Stream) { closed <- st }),
	})
	guest := NewSession(clientConn, Config{Role: RoleGuest, Handler: handlerFunc(
		func(context.Context, string, []byte) (Endpoint, []byte, error) {
			return nil, nil, errors.New("guest handler must not be used")
		})})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = host.Serve(ctx) }()
	go func() { _ = guest.Serve(ctx) }()
	defer host.Close()
	defer guest.Close()

	st, err := guest.Open(context.Background(), "svc", nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ep := <-eps
	payload := bytes.Repeat([]byte{0x5A}, 128<<10)
	if _, err := st.Write(payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := st.CloseWrite(); err != nil {
		t.Fatalf("close write: %v", err)
	}

	premature := false
	select {
	case <-closed:
		premature = true // both sides closed while inbound data is buffered
	case <-time.After(300 * time.Millisecond):
	}
	ep.release()
	if !premature {
		select {
		case <-closed:
		case <-time.After(5 * time.Second):
			t.Fatal("stream did not finish after the endpoint drained")
		}
	}
	if got := ep.data(); !bytes.Equal(got, payload) {
		t.Fatalf("endpoint received %d bytes, want %d (clean close must not truncate)", len(got), len(payload))
	}
}
