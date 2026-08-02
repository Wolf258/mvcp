package protocol

import (
	"encoding/binary"
	"io"
)

func WriteUint8(w io.Writer, v uint8) error {
	_, err := w.Write([]byte{v})
	return err
}

func WriteUint16(w io.Writer, v uint16) error {
	var buf [2]byte
	binary.BigEndian.PutUint16(buf[:], v)
	_, err := w.Write(buf[:])
	return err
}

func WriteUint32(w io.Writer, v uint32) error {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], v)
	_, err := w.Write(buf[:])
	return err
}

func WriteUint64(w io.Writer, v uint64) error {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	_, err := w.Write(buf[:])
	return err
}

func WriteInt32(w io.Writer, v int32) error {
	return WriteUint32(w, uint32(v))
}

func WriteInt64(w io.Writer, v int64) error {
	return WriteUint64(w, uint64(v))
}

func WriteBool(w io.Writer, v bool) error {
	if v {
		return WriteUint8(w, 1)
	}
	return WriteUint8(w, 0)
}

func WriteString(w io.Writer, s string) error {
	if err := WriteUint16(w, uint16(len(s))); err != nil {
		return err
	}
	_, err := io.WriteString(w, s)
	return err
}

func WriteBytes(w io.Writer, b []byte) error {
	if err := WriteUint32(w, uint32(len(b))); err != nil {
		return err
	}
	_, err := w.Write(b)
	return err
}

func WriteStringSlice(w io.Writer, s []string) error {
	if err := WriteUint16(w, uint16(len(s))); err != nil {
		return err
	}
	for _, v := range s {
		if err := WriteString(w, v); err != nil {
			return err
		}
	}
	return nil
}

func WriteStringMap(w io.Writer, m map[string]string) error {
	if err := WriteUint16(w, uint16(len(m))); err != nil {
		return err
	}
	for k, v := range m {
		if err := WriteString(w, k); err != nil {
			return err
		}
		if err := WriteString(w, v); err != nil {
			return err
		}
	}
	return nil
}
