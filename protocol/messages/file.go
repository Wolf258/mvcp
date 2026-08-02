package messages

import (
	"bytes"
	"fmt"
	"io"

	"github.com/Wolf258/mvcp/protocol"
)

type XferInit struct {
	Path      string
	TotalSize uint32
	Dir       uint8
}

func (m *XferInit) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	protocol.WriteString(&buf, m.Path)
	protocol.WriteUint32(&buf, m.TotalSize)
	protocol.WriteUint8(&buf, m.Dir)
	return buf.Bytes(), nil
}

func (m *XferInit) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	path, err := protocol.ReadString(r)
	if err != nil {
		return fmt.Errorf("xfer_init: read path: %w", err)
	}
	size, err := protocol.ReadUint32(r)
	if err != nil {
		return fmt.Errorf("xfer_init: read total_size: %w", err)
	}
	dir, err := protocol.ReadUint8(r)
	if err != nil {
		return fmt.Errorf("xfer_init: read dir: %w", err)
	}
	m.Path = path
	m.TotalSize = size
	m.Dir = dir
	return nil
}

type XferChunk struct {
	Seq  uint32
	Data []byte
}

func (m *XferChunk) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	protocol.WriteUint32(&buf, m.Seq)
	protocol.WriteBytes(&buf, m.Data)
	return buf.Bytes(), nil
}

func (m *XferChunk) UnmarshalBinary(b []byte) error {
	r := bytes.NewReader(b)
	seq, err := protocol.ReadUint32(r)
	if err != nil {
		return fmt.Errorf("xfer_chunk: read seq: %w", err)
	}
	chunkData, err := protocol.ReadBytes(r)
	if err != nil {
		return fmt.Errorf("xfer_chunk: read data: %w", err)
	}
	m.Seq = seq
	m.Data = chunkData
	return nil
}

type XferDone struct {
	Ok             bool
	ChunksReceived uint32
	BytesWritten   uint64
}

func (m *XferDone) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	protocol.WriteBool(&buf, m.Ok)
	protocol.WriteUint32(&buf, m.ChunksReceived)
	protocol.WriteUint64(&buf, m.BytesWritten)
	return buf.Bytes(), nil
}

func (m *XferDone) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	ok, err := protocol.ReadBool(r)
	if err != nil {
		return fmt.Errorf("xfer_done: read ok: %w", err)
	}
	chunks, err := protocol.ReadUint32(r)
	if err != nil {
		return fmt.Errorf("xfer_done: read chunks_received: %w", err)
	}
	written, err := protocol.ReadUint64(r)
	if err != nil {
		return fmt.Errorf("xfer_done: read bytes_written: %w", err)
	}
	m.Ok = ok
	m.ChunksReceived = chunks
	m.BytesWritten = written
	return nil
}

func init() {
	protocol.RegisterMessage(protocol.TypeXFERINIT, func(r io.Reader) (protocol.Message, error) {
		path, err := protocol.ReadString(r)
		if err != nil {
			return nil, fmt.Errorf("xfer_init: %w", err)
		}
		size, err := protocol.ReadUint32(r)
		if err != nil {
			return nil, fmt.Errorf("xfer_init: %w", err)
		}
		dir, err := protocol.ReadUint8(r)
		if err != nil {
			return nil, fmt.Errorf("xfer_init: %w", err)
		}
		return &XferInit{Path: path, TotalSize: size, Dir: dir}, nil
	})
	protocol.RegisterMessage(protocol.TypeXFERCHUNK, func(r io.Reader) (protocol.Message, error) {
		seq, err := protocol.ReadUint32(r)
		if err != nil {
			return nil, fmt.Errorf("xfer_chunk: %w", err)
		}
		data, err := protocol.ReadBytes(r)
		if err != nil {
			return nil, fmt.Errorf("xfer_chunk: %w", err)
		}
		return &XferChunk{Seq: seq, Data: data}, nil
	})
	protocol.RegisterMessage(protocol.TypeXFERDONE, func(r io.Reader) (protocol.Message, error) {
		ok, err := protocol.ReadBool(r)
		if err != nil {
			return nil, fmt.Errorf("xfer_done: %w", err)
		}
		chunks, err := protocol.ReadUint32(r)
		if err != nil {
			return nil, fmt.Errorf("xfer_done: %w", err)
		}
		written, err := protocol.ReadUint64(r)
		if err != nil {
			return nil, fmt.Errorf("xfer_done: %w", err)
		}
		return &XferDone{Ok: ok, ChunksReceived: chunks, BytesWritten: written}, nil
	})
}
