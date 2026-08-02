package rpc

import (
	"context"
	"io"
	"log"
	"sync"

	"github.com/Wolf258/mvcp/protocol"
)

type Handler func(ctx context.Context, req *Request) error

type Server struct {
	conn     io.ReadWriter
	handlers map[uint8]Handler
	mu       sync.RWMutex
}

func NewServer(conn io.ReadWriter) *Server {
	return &Server{
		conn:     conn,
		handlers: make(map[uint8]Handler),
	}
}

func (s *Server) Handle(msgType uint8, h Handler) {
	s.mu.Lock()
	s.handlers[msgType] = h
	s.mu.Unlock()
}

func (s *Server) Serve(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		frame, err := protocol.ReadMVCPFrame(s.conn)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}

		if frame.Flags&protocol.FlagResponse != 0 {
			continue
		}

		s.mu.RLock()
		handler, ok := s.handlers[frame.Type]
		s.mu.RUnlock()

		if !ok {
			s.sendError(frame.MsgID, protocol.ErrorCodeUnknownType, "unknown message type")
			continue
		}

		req := &Request{
			Type:  frame.Type,
			MsgID: frame.MsgID,
			Flags: frame.Flags,
			Body:  frame.Body,
			conn:  s.conn,
		}

		go func() {
			if err := handler(ctx, req); err != nil {
				log.Printf("mvcp rpc: handler for type 0x%02X returned: %v", frame.Type, err)
			}
		}()
	}
}

func (s *Server) sendError(msgID uint32, code uint16, message string) {
	req := &Request{
		MsgID: msgID,
		conn:  s.conn,
	}
	req.Error(code, message)
}
