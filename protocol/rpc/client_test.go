package rpc

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"github.com/Wolf258/mvcp/protocol"
)

// startTestServer wires an rpc.Server on one end of net.Pipe and a Client
// on the other end.
func startTestServer(t *testing.T, register func(*Server)) (*Client, context.CancelFunc) {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	srv := NewServer(serverConn)
	register(srv)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Serve(ctx) }()
	client := NewClient(clientConn)
	client.Start(ctx)
	t.Cleanup(func() {
		cancel()
		// Close the server end first: it unblocks the client's readLoop
		// so client.Close does not wait on a silent peer (net.Pipe read
		// only returns once the other end is closed).
		_ = serverConn.Close()
		_ = client.Close()
	})
	return client, cancel
}

func TestRequestStartedWritesFlagsZero(t *testing.T) {
	var buf bytes.Buffer
	req := &Request{Type: protocol.TypeEXEC, MsgID: 7, conn: &buf}
	if err := req.Started(false); err != nil {
		t.Fatalf("Started: %v", err)
	}
	frame, err := protocol.ReadMVCPFrame(&buf)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if frame.Type != protocol.TypeSTARTED || frame.Flags != 0 || frame.MsgID != 7 {
		t.Fatalf("frame = type 0x%02X flags 0x%02X msg_id %d, want STARTED/flags=0/7",
			frame.Type, frame.Flags, frame.MsgID)
	}
}

func TestCallSkipsStartedAndReturnsFinal(t *testing.T) {
	client, _ := startTestServer(t, func(srv *Server) {
		srv.Handle(protocol.TypePING, func(ctx context.Context, req *Request) error {
			if err := req.Started(false); err != nil {
				return err
			}
			return req.Respond(protocol.TypePONG, []byte("pong"))
		})
	})
	resp, err := client.Call(context.Background(), protocol.TypePING, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Type != protocol.TypePONG || string(resp.Body) != "pong" {
		t.Fatalf("response = type 0x%02X body %q, want PONG/pong", resp.Type, resp.Body)
	}
}

func TestStreamDeliversStartedFirst(t *testing.T) {
	client, _ := startTestServer(t, func(srv *Server) {
		srv.Handle(protocol.TypeEXEC, func(ctx context.Context, req *Request) error {
			if err := req.Started(true); err != nil {
				return err
			}
			return req.Respond(protocol.TypeEXECRESULT, []byte("done"))
		})
	})
	ch, err := client.Stream(context.Background(), protocol.TypeEXEC, nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var frames []*StreamFrame
	for f := range ch {
		frames = append(frames, f)
	}
	if len(frames) != 2 || frames[0].Type != protocol.TypeSTARTED {
		t.Fatalf("frames = %d first %v, want 2 with STARTED first", len(frames), frames)
	}
}

func TestStreamSlowConsumerDoesNotLoseFrames(t *testing.T) {
	const total = 40
	client, _ := startTestServer(t, func(srv *Server) {
		srv.Handle(protocol.TypePING, func(ctx context.Context, req *Request) error {
			for i := 0; i < total; i++ {
				if err := req.Stream(protocol.TypePONG, []byte{byte(i)}); err != nil {
					return err
				}
			}
			return req.Respond(protocol.TypePONG, []byte("end"))
		})
	})
	ch, err := client.Stream(context.Background(), protocol.TypePING, nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	// Slow consumer: stall while the server produces more than the
	// internal buffer, then drain. The old non-blocking send would drop
	// frames silently.
	time.Sleep(100 * time.Millisecond)
	var got []byte
	var end bool
	for f := range ch {
		if !f.More {
			end = string(f.Body) == "end"
			continue
		}
		got = append(got, f.Body...)
	}
	if len(got) != total {
		t.Fatalf("received %d frames, want %d (silent frame loss)", len(got), total)
	}
	for i, b := range got {
		if int(b) != i {
			t.Fatalf("frame %d = %d, order broken", i, b)
		}
	}
	if !end {
		t.Fatal("did not receive final response frame")
	}
}

func TestAbandonedStreamUnblocksReadLoop(t *testing.T) {
	const total = 64 // > inbox(16) + out(16): the read loop blocks in send
	client, _ := startTestServer(t, func(srv *Server) {
		srv.Handle(protocol.TypeEXEC, func(ctx context.Context, req *Request) error {
			for i := 0; i < total; i++ {
				if err := req.Stream(protocol.TypePONG, []byte{byte(i)}); err != nil {
					return nil // stream abandoned: stop producing
				}
			}
			return req.Respond(protocol.TypePONG, []byte("done"))
		})
		srv.Handle(protocol.TypePING, func(ctx context.Context, req *Request) error {
			return req.Respond(protocol.TypePONG, []byte("ok"))
		})
	})
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := client.Stream(ctx, protocol.TypeEXEC, nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	// Consume one frame, then stop draining: the buffers fill and the
	// read loop blocks in send, which is the state this test must escape.
	<-ch
	time.Sleep(50 * time.Millisecond)
	cancel()
	// The readLoop must be unblocked by the abandonment and keep serving
	// the same connection.
	callCtx, callCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer callCancel()
	resp, err := client.Call(callCtx, protocol.TypePING, nil)
	if err != nil {
		t.Fatalf("Call after abandoning a blocked stream: %v", err)
	}
	if resp.Type != protocol.TypePONG || string(resp.Body) != "ok" {
		t.Fatalf("response = type 0x%02X body %q, want PONG/ok", resp.Type, resp.Body)
	}
}
