package network

import (
	"encoding/binary"
	"io"
)

func writeLengthPrefix(w io.Writer, data []byte) error {
	buf := make([]byte, 4+len(data))

	binary.BigEndian.PutUint32(buf[:4], uint32(len(data)))

	copy(buf[4:], data)

	_, err := w.Write(buf)
	return err
}

func readLengthPrefix(r io.Reader) ([]byte, error) {
	header := make([]byte, 4)

	_, err := io.ReadFull(r, header)
	if err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(header)

	payload := make([]byte, length)

	_, err = io.ReadFull(r, payload)
	if err != nil {
		return nil, err
	}

	return payload, nil
}
