package rpc

import (
	"context"
	"errors"
	"io"
	"log"
	"sync"

	"github.com/Wolf258/mvcp/protocol"
)

type pendingCall struct {
	mu     sync.Mutex
	stream chan *StreamFrame
	closed bool
}

func (p *pendingCall) closeStream() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed {
		p.closed = true
		close(p.stream)
	}
}

func (p *pendingCall) send(f *StreamFrame) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return false
	}
	select {
	case p.stream <- f:
		return true
	default:
		return false
	}
}

type Client struct {
	conn    io.ReadWriter
	mu      sync.Mutex
	msgID   uint32
	pending map[uint32]*pendingCall
	closed  bool
	closeCh chan struct{}
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func NewClient(conn io.ReadWriter) *Client {
	return &Client{
		conn:    conn,
		msgID:   0,
		pending: make(map[uint32]*pendingCall),
		closeCh: make(chan struct{}),
	}
}

func (c *Client) Start(ctx context.Context) {
	c.mu.Lock()
	if c.cancel != nil {
		c.mu.Unlock()
		return
	}
	readerCtx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.mu.Unlock()

	c.wg.Add(1)
	go c.readLoop(readerCtx)

	go func() {
		<-ctx.Done()
		c.shutdown()
	}()
}

func (c *Client) shutdown() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	close(c.closeCh)
	if c.cancel != nil {
		c.cancel()
	}
	for _, p := range c.pending {
		p.closeStream()
	}
	c.pending = make(map[uint32]*pendingCall)
	c.mu.Unlock()
	c.wg.Wait()
}

func (c *Client) Close() error {
	c.shutdown()
	return c.conn.(io.Closer).Close()
}

// IsClosed reports whether the client has shut down — because the context
// passed to Start was canceled, the connection failed, or Close was called.
// A closed client rejects all future calls and must not be reused.
func (c *Client) IsClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *Client) allocMsgID() uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgID++
	if c.msgID == 0 {
		c.msgID = 1
	}
	return c.msgID
}

func (c *Client) addPending(msgID uint32) *pendingCall {
	p := &pendingCall{stream: make(chan *StreamFrame, 16)}
	c.mu.Lock()
	c.pending[msgID] = p
	c.mu.Unlock()
	return p
}

func (c *Client) removePending(msgID uint32) {
	c.mu.Lock()
	delete(c.pending, msgID)
	c.mu.Unlock()
}

func (c *Client) writeRequest(msgID uint32, msgType uint8, flags uint8, body []byte) error {
	return protocol.WriteMVCPFrame(c.conn, &protocol.Frame{
		Type:  msgType,
		Flags: flags,
		MsgID: msgID,
		Body:  body,
	})
}

func (c *Client) readLoop(ctx context.Context) {
	defer c.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		frame, err := protocol.ReadMVCPFrame(c.conn)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if !errors.Is(err, io.EOF) {
				log.Printf("mvcp rpc: read frame: %v", err)
			}
			c.shutdown()
			return
		}

		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return
		}
		p := c.pending[frame.MsgID]
		c.mu.Unlock()

		if p == nil {
			continue
		}

		if frame.Flags&protocol.FlagResponse != 0 {
			p.send(&StreamFrame{Type: frame.Type, Body: frame.Body, More: false})
			p.closeStream()
			c.removePending(frame.MsgID)
			continue
		}

		p.send(&StreamFrame{Type: frame.Type, Body: frame.Body, More: true})
	}
}

func (c *Client) Call(ctx context.Context, msgType uint8, body []byte) (*Response, error) {
	return c.CallFlags(ctx, msgType, 0, body)
}

func (c *Client) CallFlags(ctx context.Context, msgType uint8, flags uint8, body []byte) (*Response, error) {
	msgID := c.allocMsgID()
	p := c.addPending(msgID)
	defer func() {
		p.closeStream()
		c.removePending(msgID)
	}()

	if err := c.writeRequest(msgID, msgType, flags, body); err != nil {
		return nil, err
	}

	select {
	case frame := <-p.stream:
		if frame == nil {
			return nil, errors.New("mvcp rpc: connection closed")
		}
		if frame.Type == protocol.TypeSTARTED {
			goto waitResponse
		}
		return &Response{Type: frame.Type, Body: frame.Body}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closeCh:
		return nil, errors.New("mvcp rpc: client closed")
	}
waitResponse:
	select {
	case frame := <-p.stream:
		if frame == nil {
			return nil, errors.New("mvcp rpc: connection closed")
		}
		return &Response{Type: frame.Type, Body: frame.Body}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closeCh:
		return nil, errors.New("mvcp rpc: client closed")
	}
}

func (c *Client) Stream(ctx context.Context, msgType uint8, body []byte) (<-chan *StreamFrame, error) {
	return c.StreamFlags(ctx, msgType, 0, body)
}

func (c *Client) StreamFlags(ctx context.Context, msgType uint8, flags uint8, body []byte) (<-chan *StreamFrame, error) {
	msgID := c.allocMsgID()
	p := c.addPending(msgID)

	if err := c.writeRequest(msgID, msgType, flags, body); err != nil {
		c.removePending(msgID)
		p.closeStream()
		return nil, err
	}

	go func() {
		select {
		case <-ctx.Done():
			p.closeStream()
			c.removePending(msgID)
		case <-c.closeCh:
			p.closeStream()
			c.removePending(msgID)
		}
	}()

	return p.stream, nil
}
