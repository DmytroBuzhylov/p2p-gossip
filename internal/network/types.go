package network

import "github.com/quic-go/quic-go"

type NewConnEvent struct {
	Conn   *quic.Conn
	IsOut  bool
	PeerID []byte
	Addr   string
}
