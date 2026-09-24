package app

import (
	"io"
	"sync"

	"github.com/Wolf258/mvcp/protocol"
	"github.com/Wolf258/mvcp/protocol/messages"
)

type Stream struct {
	sess    *Session
	id      uint32
	service string
	inbound bool

	mu   sync.Mutex
	cond *sync.Cond

	// lifecycle
	opening      bool
	accepting    bool
	localClosed  bool // we sent APP_CLOSE
	remoteClosed bool
	reset        bool
	resetErr     *ResetError
	ready        chan struct{}
	openErr      error

	// lifeMu serializes opened/finished transitions and observer
	// notifications so the observer always sees StreamOpened before
	// StreamClosed (and never one without the other).
	lifeMu   sync.Mutex
	opened   bool // StreamOpened notification sent
	finished bool // finish() ran (detach/observer exactly once)

	// flow control
	sndCredit       uint32
	rcvWindow       uint32
	sendQ           [][]byte
	sendBytes       uint64
	inQ             [][]byte
	inBytes         uint64
	closeAfterFlush bool

	bytesIn, bytesOut uint64
	acceptMeta        []byte
	ep                Endpoint
}

func newStream(s *Session, id uint32, service string, inbound bool) *Stream {
	st := &Stream{sess: s, id: id, service: service, inbound: inbound, ready: make(chan struct{})}
	st.cond = sync.NewCond(&st.mu)
	return st
}

func (st *Stream) ID() uint32         { return st.id }
func (st *Stream) Service() string    { return st.service }
func (st *Stream) Inbound() bool      { return st.inbound }
func (st *Stream) AcceptMeta() []byte { return append([]byte(nil), st.acceptMeta...) }

func (st *Stream) BytesIn() uint64 {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.bytesIn
}

func (st *Stream) BytesOut() uint64 {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.bytesOut
}

// terminalError is nil for a clean close or the ResetError otherwise.
func (st *Stream) terminalError() error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.resetErr != nil {
		return st.resetErr
	}
	return nil
}

// markOpened transitions the stream to open and notifies the observer
// exactly once. It is serialized with finish so the observer never sees
// StreamClosed before StreamOpened.
func (st *Stream) markOpened() {
	st.lifeMu.Lock()
	defer st.lifeMu.Unlock()
	if st.finished || st.opened {
		return
	}
	st.opened = true
	if st.sess.cfg.Observer != nil {
		st.sess.cfg.Observer.StreamOpened(st)
	}
}

func (st *Stream) attachEndpoint(ep Endpoint) {
	st.mu.Lock()
	st.ep = ep
	st.mu.Unlock()
}

func (st *Stream) closeEndpoint() {
	st.mu.Lock()
	ep := st.ep
	st.ep = nil
	st.mu.Unlock()
	if ep != nil {
		_ = ep.Close()
	}
}

func (st *Stream) startWriteLoop() {
	s := st.sess
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		st.writeLoop()
	}()
}

func (st *Stream) writeLoop() {
	for {
		st.mu.Lock()
		for {
			if st.reset {
				st.mu.Unlock()
				return
			}
			if len(st.sendQ) > 0 && st.sndCredit > 0 {
				break
			}
			if len(st.sendQ) == 0 && st.closeAfterFlush {
				st.localClosed = true
				st.mu.Unlock()
				if err := st.sess.sendApp(&messages.AppClose{StreamID: st.id}); err != nil {
					st.sess.fail(err)
					return
				}
				st.maybeFinish()
				return
			}
			st.cond.Wait()
		}
		n := uint32(len(st.sendQ[0]))
		if n > st.sndCredit {
			n = st.sndCredit
		}
		if n > protocol.AppMaxDataBytes {
			n = protocol.AppMaxDataBytes
		}
		data := st.sendQ[0][:n]
		st.sndCredit -= n
		st.sendBytes -= uint64(n)
		st.bytesOut += uint64(n)
		if uint32(len(st.sendQ[0])) == n {
			st.sendQ = st.sendQ[1:]
		} else {
			st.sendQ[0] = st.sendQ[0][n:]
		}
		st.mu.Unlock()
		st.sess.releaseBuffer(uint64(n))
		if err := st.sess.sendApp(&messages.AppData{StreamID: st.id, Data: data}); err != nil {
			st.sess.fail(err)
			return
		}
	}
}

// Write queues p (up to AppMaxSendQueue) and returns; the write loop
// sends as credit allows. Overflow resets the stream, never drops.
func (st *Stream) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	st.mu.Lock()
	if st.reset {
		err := st.resetErr
		st.mu.Unlock()
		return 0, err
	}
	if st.localClosed || st.closeAfterFlush {
		st.mu.Unlock()
		return 0, ErrStreamClosed
	}
	if st.sendBytes+uint64(len(p)) > protocol.AppMaxSendQueue {
		st.mu.Unlock()
		st.resetRemote(protocol.ErrorCodeAppOverflow, "send queue overflow")
		return 0, &ResetError{Code: protocol.ErrorCodeAppOverflow, Message: "send queue overflow"}
	}
	if !st.sess.reserveBuffer(uint64(len(p))) {
		st.mu.Unlock()
		st.resetRemote(protocol.ErrorCodeAppQuotaExceeded, "session buffer exceeded")
		return 0, &ResetError{Code: protocol.ErrorCodeAppQuotaExceeded, Message: "session buffer exceeded"}
	}
	chunk := append([]byte(nil), p...)
	st.sendQ = append(st.sendQ, chunk)
	st.sendBytes += uint64(len(p))
	st.cond.Broadcast()
	st.mu.Unlock()
	return len(p), nil
}

// Read returns buffered inbound bytes, io.EOF after the peer's APP_CLOSE
// with no data left, or the ResetError.
func (st *Stream) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	st.mu.Lock()
	for {
		if st.reset {
			err := st.resetErr
			st.mu.Unlock()
			return 0, err
		}
		if len(st.inQ) > 0 {
			break
		}
		if st.remoteClosed {
			st.mu.Unlock()
			return 0, io.EOF
		}
		st.cond.Wait()
	}
	n := copy(p, st.inQ[0])
	st.inQ[0] = st.inQ[0][n:]
	if len(st.inQ[0]) == 0 {
		st.inQ = st.inQ[1:]
	}
	st.inBytes -= uint64(n)
	st.bytesIn += uint64(n)
	st.rcvWindow += uint32(n)
	st.mu.Unlock()
	st.sess.releaseBuffer(uint64(n))
	if err := st.sess.sendApp(&messages.AppCredit{StreamID: st.id, Bytes: uint32(n)}); err != nil {
		st.sess.fail(err)
	}
	return n, nil
}

// CloseWrite flushes queued data and sends APP_CLOSE (half-close).
func (st *Stream) CloseWrite() error {
	st.mu.Lock()
	if st.reset {
		err := st.resetErr
		st.mu.Unlock()
		return err
	}
	if st.localClosed || st.closeAfterFlush {
		st.mu.Unlock()
		return nil
	}
	st.closeAfterFlush = true
	if len(st.sendQ) == 0 {
		st.localClosed = true
		st.mu.Unlock()
		if err := st.sess.sendApp(&messages.AppClose{StreamID: st.id}); err != nil {
			st.sess.fail(err)
			return err
		}
		st.maybeFinish()
		return nil
	}
	st.cond.Broadcast()
	st.mu.Unlock()
	return nil
}

// Close closes the local direction (half-close). The stream is released
// when the peer also closes or the session ends.
func (st *Stream) Close() error { return st.CloseWrite() }

// Reset terminates both directions immediately with an explicit reason.
func (st *Stream) Reset(code uint16, message string) {
	st.resetRemote(code, message)
}

func (st *Stream) deliver(data []byte) {
	if len(data) == 0 {
		return // empty APP_DATA is a no-op (never produces a 0-byte Read)
	}
	st.mu.Lock()
	if st.accepting || st.opening {
		st.mu.Unlock()
		st.resetRemote(protocol.ErrorCodeAppProtocolError, "data before accept")
		return
	}
	if st.remoteClosed || st.reset {
		st.mu.Unlock()
		st.resetRemote(protocol.ErrorCodeAppProtocolError, "data after close")
		return
	}
	if uint32(len(data)) > st.rcvWindow {
		st.mu.Unlock()
		st.resetRemote(protocol.ErrorCodeAppProtocolError, "credit exceeded")
		return
	}
	if !st.sess.reserveBuffer(uint64(len(data))) {
		st.mu.Unlock()
		st.resetRemote(protocol.ErrorCodeAppQuotaExceeded, "session buffer exceeded")
		return
	}
	st.rcvWindow -= uint32(len(data))
	st.inQ = append(st.inQ, data)
	st.inBytes += uint64(len(data))
	st.cond.Broadcast()
	st.mu.Unlock()
}

func (st *Stream) addCredit(n uint32) {
	if n == 0 {
		return
	}
	st.mu.Lock()
	if st.reset {
		st.mu.Unlock()
		return
	}
	if st.sndCredit+n > protocol.AppMaxCredit {
		st.mu.Unlock()
		st.resetRemote(protocol.ErrorCodeAppProtocolError, "credit cap exceeded")
		return
	}
	st.sndCredit += n
	st.cond.Broadcast()
	st.mu.Unlock()
}

func (st *Stream) remoteClose() {
	st.mu.Lock()
	if st.reset || st.remoteClosed {
		st.mu.Unlock()
		return
	}
	st.remoteClosed = true
	st.cond.Broadcast()
	st.mu.Unlock()
	st.maybeFinish()
}

func (st *Stream) remoteReset(code uint16, message string) {
	st.setReset(code, message)
	st.sess.finish(st)
}

// resetRemote notifies the peer with APP_RESET and tears the stream down.
func (st *Stream) resetRemote(code uint16, message string) {
	if err := st.sess.sendApp(&messages.AppReset{StreamID: st.id, Code: code, Message: message}); err != nil {
		st.sess.fail(err)
	}
	st.setReset(code, message)
	st.sess.finish(st)
}

func (st *Stream) setReset(code uint16, message string) {
	st.mu.Lock()
	if !st.reset {
		st.reset = true
		st.resetErr = &ResetError{Code: code, Message: message}
		if st.opening {
			st.opening = false
			close(st.ready) // unblock Open on reset during the open handshake
		}
	}
	st.mu.Unlock()
	st.cond.Broadcast()
}

func (st *Stream) maybeFinish() {
	st.mu.Lock()
	done := st.localClosed && st.remoteClosed && len(st.sendQ) == 0
	st.mu.Unlock()
	if done {
		st.sess.finish(st)
	}
}
