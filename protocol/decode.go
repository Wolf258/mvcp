package protocol

import (
	"encoding/binary"
	"io"
)

func ReadUint8(r io.Reader) (uint8, error) {
	var buf [1]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return buf[0], nil
}

func ReadUint16(r io.Reader) (uint16, error) {
	var buf [2]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(buf[:]), nil
}

func ReadUint32(r io.Reader) (uint32, error) {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(buf[:]), nil
}

func ReadUint64(r io.Reader) (uint64, error) {
	var buf [8]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(buf[:]), nil
}

func ReadInt32(r io.Reader) (int32, error) {
	v, err := ReadUint32(r)
	return int32(v), err
}

func ReadInt64(r io.Reader) (int64, error) {
	v, err := ReadUint64(r)
	return int64(v), err
}

func ReadBool(r io.Reader) (bool, error) {
	v, err := ReadUint8(r)
	if err != nil {
		return false, err
	}
	return v != 0, nil
}

func ReadString(r io.Reader) (string, error) {
	length, err := ReadUint16(r)
	if err != nil {
		return "", err
	}
	if length == 0 {
		return "", nil
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func ReadBytes(r io.Reader) ([]byte, error) {
	length, err := ReadUint32(r)
	if err != nil {
		return nil, err
	}
	if length == 0 {
		return nil, nil
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func ReadStringSlice(r io.Reader) ([]string, error) {
	count, err := ReadUint16(r)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}
	s := make([]string, count)
	for i := uint16(0); i < count; i++ {
		v, err := ReadString(r)
		if err != nil {
			return nil, err
		}
		s[i] = v
	}
	return s, nil
}

func ReadStringMap(r io.Reader) (map[string]string, error) {
	count, err := ReadUint16(r)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}
	m := make(map[string]string, count)
	for i := uint16(0); i < count; i++ {
		k, err := ReadString(r)
		if err != nil {
			return nil, err
		}
		v, err := ReadString(r)
		if err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, nil
}
