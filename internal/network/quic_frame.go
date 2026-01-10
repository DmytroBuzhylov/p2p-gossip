package network

import "io"

func writeFrame(w io.Writer, msgType MessageType, data []byte) error {

	payload := make([]byte, 1+len(data))

	copy(payload[1:], data)

	payload[0] = byte(msgType)

	return writeLengthPrefix(w, payload)
}

func readFrame(r io.Reader) (MessageType, []byte, error) {
	data, err := readLengthPrefix(r)
	if err != nil {
		return TypeUnknown, nil, err
	}
	msgType := MessageType(data[0])

	return msgType, data[1:], err
}
