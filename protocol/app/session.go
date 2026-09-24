// Package app implements the MVCP app channel (port 9005): multiplexed,
// credit-controlled bidirectional byte streams over one persistent
// connection. The wire contract lives in docs/services/app-channel.md;
// core and vhandler both terminate the protocol through this package.
package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/Wolf258/mvcp/protocol"
	"github.com/Wolf258/mvcp/protocol/messages"
)

// Role identifies which side of the VM boundary this session runs on.
// It fixes the initiator bit (bit 31) of locally allocated stream IDs:
// host/core uses 0, guest/vhandler uses 1.
type Role uint8

const (
	RoleHost  Role = 0
	RoleGuest Role = 1
)

// Endpoint is the local side of one stream.
type Endpoint interface {
	io.Reader
	io.Writer
	io.Closer
}

// Handler serves remote-initiated streams. Open returns the local
// endpoint and the meta echoed back in APP_ACCEPT. Returning a
// *RejectError rejects the stream with its wire code; any other error
// maps to APP_LOCAL_ERROR.
type Handler interface {
	Open(ctx context.Context, service string, meta []byte) (Endpoint, []byte, error)
}

// Observer receives stream lifecycle callbacks. Optional.
type Observer interface {
	StreamOpened(st *Stream)
	StreamClosed(st *Stream)
}

// Config configures a Session.
type Config struct {
	Role     Role
	Handler  Handler
	Observer Observer
	Logf     func(format string, args ...any)
}

// RejectError rejects a remote open with an explicit wire code.
type RejectError struct {
	Code    uint16
	Message string
}

func (e *RejectError) Error() string {
	return fmt.Sprintf("app: rejected 0x%04X: %s", e.Code, e.Message)
}

// ResetError reports a stream terminated by APP_RESET or connection loss.
type ResetError struct {
	Code    uint16
	Message string
}

func (e *ResetError) Error() string {
	return fmt.Sprintf("app: reset 0x%04X: %s", e.Code, e.Message)
}

var (
	ErrSessionClosed = errors.New("app: session closed")
	ErrStreamClosed  = errors.New("app: stream closed")
)

// ErrorName returns the short code name used by the guest control socket
// protocol (docs/services/app-channel.md §Guest contract).
func ErrorName(code uint16) string {
	switch code {
	case protocol.ErrorCodeAppServiceNotFound:
		return "SERVICE_NOT_FOUND"
	case protocol.ErrorCodeAppNotAuthorized:
		return "NOT_AUTHORIZED"
	case protocol.ErrorCodeAppServiceBusy:
		return "SERVICE_BUSY"
	case protocol.ErrorCodeAppQuotaExceeded:
		return "QUOTA_EXCEEDED"
	case protocol.ErrorCodeAppPeerGone:
		return "PEER_GONE"
	case protocol.ErrorCodeAppOverflow:
		return "OVERFLOW"
	case protocol.ErrorCodeAppTimeout:
		return "TIMEOUT"
	case protocol.ErrorCodeAppProtocolError:
		return "PROTOCOL_ERROR"
	default:
		return "LOCAL_ERROR"
	}
}

// Session multiplexes app streams over one MVCP connection.
//
// Locking: Session.mu guards the stream table and closed flag; bufMu
// guards the buffered-byte ledger. Stream.mu guards per-stream state.
// The lock order is Session.mu -> Stream.mu; stream code never calls
// Session methods that take Session.mu while holding Stream.mu (buffer
// accounting uses bufMu and is safe).
type Session struct {
	conn io.ReadWriter
	cfg  Config

	writeMu sync.Mutex

	mu      sync.Mutex
	streams map[uint32]*Stream
	nextID  uint32
	closed  bool
	closeCh chan struct{}

	bufMu    sync.Mutex
	buffered uint64

	wg sync.WaitGroup
}

func NewSession(conn io.ReadWriter, cfg Config) *Session {
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	return &Session{
		conn:    conn,
		cfg:     cfg,
		streams: make(map[uint32]*Stream),
		closeCh: make(chan struct{}),
	}
}

func (s *Session) localBit() uint32  { return uint32(s.cfg.Role) << 31 }
func (s *Session) remoteBit() uint32 { return uint32(1-s.cfg.Role) << 31 }

func (s *Session) stream(id uint32) *Stream {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.streams[id]
}

func (s *Session) send(m protocol.Message, typ uint8) error {
	body, err := m.MarshalBinary()
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return protocol.WriteMVCPFrame(s.conn, &protocol.Frame{Type: typ, MsgID: 0, Body: body})
}

func (s *Session) sendApp(m protocol.Message) error {
	typ, err := appMessageType(m)
	if err != nil {
		return err
	}
	return s.send(m, typ)
}

func (s *Session) sendError(code uint16, message string) {
	var buf bytes.Buffer
	protocol.WriteUint16(&buf, code)
	protocol.WriteString(&buf, message)
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = protocol.WriteMVCPFrame(s.conn, &protocol.Frame{Type: protocol.TypeERROR, Flags: protocol.FlagResponse, MsgID: 0, Body: buf.Bytes()})
}

func (s *Session) sendReject(streamID uint32, code uint16, message string) {
	_ = s.sendApp(&messages.AppReject{StreamID: streamID, Code: code, Message: message})
}

func appMessageType(m protocol.Message) (uint8, error) {
	switch m.(type) {
	case *messages.AppOpen:
		return protocol.TypeAPPOPEN, nil
	case *messages.AppAccept:
		return protocol.TypeAPPACCEPT, nil
	case *messages.AppReject:
		return protocol.TypeAPPREJECT, nil
	case *messages.AppData:
		return protocol.TypeAPPDATA, nil
	case *messages.AppCredit:
		return protocol.TypeAPPCREDIT, nil
	case *messages.AppClose:
		return protocol.TypeAPPCLOSE, nil
	case *messages.AppReset:
		return protocol.TypeAPPRESET, nil
	default:
		return 0, fmt.Errorf("app: unknown message %T", m)
	}
}

// Serve reads frames until the connection fails, the context is
// cancelled, or a protocol violation forces a close. It returns the
// terminal error.
func (s *Session) Serve(ctx context.Context) error {
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			_ = s.Close()
		case <-stop:
		}
	}()

	for {
		frame, err := protocol.ReadMVCPFrame(s.conn)
		if err != nil {
			s.fail(err)
			return err
		}
		if err := s.handleFrame(ctx, frame); err != nil {
			s.fail(err)
			return err
		}
	}
}

func (s *Session) handleFrame(ctx context.Context, f *protocol.Frame) error {
	if f.Flags != 0 || f.MsgID != 0 {
		s.sendError(protocol.ErrorCodeBadPayload, "app: flags and msg_id must be zero")
		return errors.New("app: frame with non-zero flags or msg_id")
	}
	msg, err := protocol.DecodeMessage(f.Type, bytes.NewReader(f.Body))
	if err != nil {
		s.sendError(protocol.ErrorCodeBadPayload, "app: decode: "+err.Error())
		return err
	}
	switch m := msg.(type) {
	case *messages.AppOpen:
		s.handleOpen(ctx, m)
	case *messages.AppAccept:
		s.handleAccept(m)
	case *messages.AppReject:
		s.handleReject(m)
	case *messages.AppData:
		if st := s.stream(m.StreamID); st != nil {
			st.deliver(m.Data)
		}
	case *messages.AppCredit:
		if st := s.stream(m.StreamID); st != nil {
			st.addCredit(m.Bytes)
		}
	case *messages.AppClose:
		if st := s.stream(m.StreamID); st != nil {
			st.remoteClose()
		}
	case *messages.AppReset:
		if st := s.stream(m.StreamID); st != nil {
			st.remoteReset(m.Code, m.Message)
		}
	default:
		s.sendError(protocol.ErrorCodeBadPayload, "app: unexpected message type")
		return fmt.Errorf("app: unexpected message type 0x%02X", f.Type)
	}
	return nil
}

func (s *Session) handleOpen(ctx context.Context, m *messages.AppOpen) {
	if m.StreamID == 0 || m.StreamID&0x80000000 != s.remoteBit() || !protocol.ValidAppService(m.Service) {
		s.sendReject(m.StreamID, protocol.ErrorCodeAppProtocolError, "invalid APP_OPEN")
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if _, dup := s.streams[m.StreamID]; dup {
		s.mu.Unlock()
		s.sendReject(m.StreamID, protocol.ErrorCodeAppProtocolError, "duplicate stream_id")
		return
	}
	if len(s.streams) >= protocol.AppMaxStreams {
		s.mu.Unlock()
		s.sendReject(m.StreamID, protocol.ErrorCodeAppQuotaExceeded, "stream quota exceeded")
		return
	}
	st := newStream(s, m.StreamID, m.Service, true)
	st.accepting = true
	s.streams[m.StreamID] = st
	s.mu.Unlock()

	go s.acceptOpen(ctx, st, m.Meta)
}

func (s *Session) acceptOpen(ctx context.Context, st *Stream, meta []byte) {
	if s.cfg.Handler == nil {
		s.detach(st)
		s.sendReject(st.id, protocol.ErrorCodeAppLocalError, "no handler configured")
		return
	}
	ep, acceptMeta, err := s.cfg.Handler.Open(ctx, st.service, meta)
	if err != nil {
		code := protocol.ErrorCodeAppLocalError
		var rej *RejectError
		if errors.As(err, &rej) {
			code = rej.Code
		}
		s.detach(st)
		s.sendReject(st.id, code, err.Error())
		return
	}
	if len(acceptMeta) > protocol.AppMaxMetaBytes {
		_ = ep.Close()
		s.detach(st)
		s.sendReject(st.id, protocol.ErrorCodeAppProtocolError, "accept meta too large")
		return
	}
	// The opener gets its credit from APP_ACCEPT, so the stream must be
	// ready to receive data before the ACCEPT hits the wire: a fast
	// opener may send APP_DATA immediately after reading it. The stream
	// is marked opened before the send as well, so a connection failure
	// in the accept window still reports StreamOpened then StreamClosed.
	st.mu.Lock()
	st.accepting = false
	st.sndCredit = protocol.AppInitialWindow // granted to us by the opener's APP_OPEN
	st.rcvWindow = protocol.AppInitialWindow // window we grant via APP_ACCEPT
	st.cond.Broadcast()
	st.mu.Unlock()
	st.markOpened()
	if err := s.sendApp(&messages.AppAccept{StreamID: st.id, Meta: acceptMeta, Grant: protocol.AppInitialWindow}); err != nil {
		_ = ep.Close()
		s.fail(err)
		return
	}
	st.attachEndpoint(ep)
	st.startWriteLoop()
	go Bridge(st, ep)
}

func (s *Session) handleAccept(m *messages.AppAccept) {
	st := s.stream(m.StreamID)
	if st == nil {
		return // normal race after reset/close: ignore
	}
	if m.Grant == 0 || m.Grant > protocol.AppMaxCredit {
		st.resetRemote(protocol.ErrorCodeAppProtocolError, "invalid accept grant")
		return
	}
	st.mu.Lock()
	if !st.opening {
		st.mu.Unlock()
		return
	}
	st.opening = false
	st.sndCredit = m.Grant
	st.acceptMeta = append([]byte(nil), m.Meta...)
	st.mu.Unlock()
	close(st.ready)
}

func (s *Session) handleReject(m *messages.AppReject) {
	st := s.stream(m.StreamID)
	if st == nil {
		return
	}
	st.mu.Lock()
	if !st.opening {
		st.mu.Unlock()
		return
	}
	st.opening = false
	st.openErr = &RejectError{Code: m.Code, Message: m.Message}
	st.mu.Unlock()
	s.detach(st)
	close(st.ready)
}

// Open starts a local stream and waits for APP_ACCEPT/APP_REJECT.
func (s *Session) Open(ctx context.Context, service string, meta []byte) (*Stream, error) {
	if !protocol.ValidAppService(service) {
		return nil, fmt.Errorf("app: invalid service %q", service)
	}
	if len(meta) > protocol.AppMaxMetaBytes {
		return nil, fmt.Errorf("app: meta too large")
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, ErrSessionClosed
	}
	if len(s.streams) >= protocol.AppMaxStreams {
		s.mu.Unlock()
		return nil, &RejectError{Code: protocol.ErrorCodeAppQuotaExceeded, Message: "stream quota exceeded"}
	}
	s.nextID++
	id := s.localBit() | s.nextID
	st := newStream(s, id, service, false)
	st.opening = true
	st.rcvWindow = protocol.AppInitialWindow // granted to the peer by our APP_OPEN
	s.streams[id] = st
	s.mu.Unlock()

	if err := s.sendApp(&messages.AppOpen{StreamID: id, Service: service, Meta: meta}); err != nil {
		s.detach(st)
		return nil, err
	}
	select {
	case <-st.ready:
	case <-s.closeCh:
		return nil, ErrSessionClosed
	case <-ctx.Done():
		st.resetRemote(protocol.ErrorCodeAppTimeout, "open context done")
		return nil, ctx.Err()
	}
	st.mu.Lock()
	openErr, resetErr := st.openErr, st.resetErr
	st.mu.Unlock()
	if openErr != nil {
		return nil, openErr
	}
	if resetErr != nil {
		return nil, resetErr
	}
	st.startWriteLoop()
	st.markOpened()
	return st, nil
}

// Close terminates the session and resets every open stream with
// PEER_GONE.
func (s *Session) Close() error {
	s.fail(ErrSessionClosed)
	s.wg.Wait()
	return nil
}

func (s *Session) fail(err error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.closeCh)
	streams := make([]*Stream, 0, len(s.streams))
	for _, st := range s.streams {
		streams = append(streams, st)
	}
	s.streams = make(map[uint32]*Stream)
	s.mu.Unlock()

	s.bufMu.Lock()
	s.buffered = 0
	s.bufMu.Unlock()
	for _, st := range streams {
		st.setReset(protocol.ErrorCodeAppPeerGone, "connection closed")
		s.finish(st)
	}
	if c, ok := s.conn.(io.Closer); ok {
		_ = c.Close()
	}
}

// detach removes st from the table and releases its buffered bytes.
func (s *Session) detach(st *Stream) {
	s.mu.Lock()
	if cur, ok := s.streams[st.id]; ok && cur == st {
		delete(s.streams, st.id)
	}
	s.mu.Unlock()
	st.mu.Lock()
	remaining := st.sendBytes + st.inBytes
	st.sendBytes, st.inBytes = 0, 0
	st.mu.Unlock()
	if remaining > 0 {
		s.releaseBuffer(remaining)
	}
}

// finish detaches, closes the endpoint and notifies the observer once.
// Streams that never completed an open are not reported as closed.
func (s *Session) finish(st *Stream) {
	st.lifeMu.Lock()
	if st.finished {
		st.lifeMu.Unlock()
		return
	}
	st.finished = true
	opened := st.opened
	if opened && s.cfg.Observer != nil {
		s.cfg.Observer.StreamClosed(st)
	}
	st.lifeMu.Unlock()
	s.detach(st)
	st.closeEndpoint()
}

func (s *Session) reserveBuffer(n uint64) bool {
	s.bufMu.Lock()
	defer s.bufMu.Unlock()
	if s.buffered+n > protocol.AppMaxBuffered {
		return false
	}
	s.buffered += n
	return true
}

func (s *Session) releaseBuffer(n uint64) {
	s.bufMu.Lock()
	defer s.bufMu.Unlock()
	if n >= s.buffered {
		s.buffered = 0
		return
	}
	s.buffered -= n
}
