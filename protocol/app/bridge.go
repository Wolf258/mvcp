package app

import (
	"errors"
	"io"
	"sync"

	"github.com/Wolf258/mvcp/protocol"
)

type closeWriter interface{ CloseWrite() error }

func closeWrite(ep Endpoint) {
	if cw, ok := ep.(closeWriter); ok {
		_ = cw.CloseWrite()
		return
	}
	_ = ep.Close()
}

// Bridge pumps bytes between a session-owned stream and its local
// endpoint until both directions end. Errors reset the stream with
// APP_LOCAL_ERROR so the peer never observes an ambiguous EOF.
func Bridge(st *Stream, ep Endpoint) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := io.Copy(st, ep); err != nil && !errors.Is(err, io.EOF) {
			st.Reset(protocol.ErrorCodeAppLocalError, err.Error())
		}
		_ = st.CloseWrite()
		closeWrite(ep)
	}()
	go func() {
		defer wg.Done()
		if _, err := io.Copy(ep, st); err != nil && !errors.Is(err, io.EOF) {
			st.Reset(protocol.ErrorCodeAppLocalError, err.Error())
		}
		closeWrite(ep)
	}()
	wg.Wait()
	_ = ep.Close()
}
