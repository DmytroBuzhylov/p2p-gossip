package network

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/quic-go/quic-go"
)

type MessageType int

const (
	TypeUnknown MessageType = iota
	TypeHandshake
	TypeGossip
	TypePing
	TypeChatMessage
	TypeDatagram
	TypeGetPeerRequest
	TypeGetPeerResponse
)

type QuicErrorCode = quic.ApplicationErrorCode

const (
	ErrCodeNoError QuicErrorCode = iota
	ErrCodeProtocolViolation
	ErrCodeSpamDetected
	ErrCodeStreamError
	ErrCodeAuthFailed
	ErrCodeNormalClose
)

func GetQuicConfig() *quic.Config {
	return &quic.Config{
		Allow0RTT:             true,
		KeepAlivePeriod:       10 * time.Second,
		MaxIdleTimeout:        30 * time.Second,
		MaxIncomingStreams:    1000,
		MaxIncomingUniStreams: 1000,
		EnableDatagrams:       true,
	}
}

type QuicTransport struct {
	tr      *quic.Transport
	tlsCfg  *tls.Config
	quicCgf *quic.Config
	privKey ed25519.PrivateKey

	connChan chan NewConnEvent
}

func NewQUICTransport(addr string, tlsCfg *tls.Config, quicCgf *quic.Config, privKey ed25519.PrivateKey) *QuicTransport {
	udpAddr, _ := net.ResolveUDPAddr("udp", addr)
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatal(err)
	}
	tr := &quic.Transport{
		Conn: udpConn,
	}

	return &QuicTransport{
		tr:       tr,
		tlsCfg:   tlsCfg,
		quicCgf:  quicCgf,
		privKey:  privKey,
		connChan: make(chan NewConnEvent, 10),
	}
}

func (q *QuicTransport) ListenEarly(ctx context.Context) error {
	ln, err := q.tr.ListenEarly(q.tlsCfg, q.quicCgf)
	if err != nil {
		return fmt.Errorf("QuicTransport Listen error: %v", err)
	}

	go q.AcceptConn(ctx, ln)

	return nil
}

func (q *QuicTransport) DialEarly(ctx context.Context, addr string) error {
	targetAddres, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}

	conn, err := q.tr.DialEarly(ctx, targetAddres, q.tlsCfg, q.quicCgf)
	if err != nil {
		return err
	}

	stream, err := conn.OpenStream()
	if err != nil {
		conn.CloseWithError(ErrCodeStreamError, "stream error")
		return err
	}

	peerID, err := q.authenticatePeer(stream)
	if err != nil {
		conn.CloseWithError(ErrCodeAuthFailed, "auth failed")
		return err
	}

	q.newConn(conn, true, conn.RemoteAddr().String(), peerID)
	fmt.Printf("Peer %s authenticated and TLS confirmed\n", peerID)

	return nil
}

func (q *QuicTransport) AcceptConn(ctx context.Context, ln *quic.EarlyListener) {
	defer ln.Close()
	for {
		conn, err := ln.Accept(ctx)
		if err != nil {
			fmt.Println(err)
			continue
		}

		go func(c *quic.Conn) {
			stream, err := c.AcceptStream(ctx)
			if err != nil {
				c.CloseWithError(ErrCodeStreamError, "stream error")
				return
			}

			peerID, err := q.authenticatePeer(stream)
			if err != nil {
				c.CloseWithError(ErrCodeAuthFailed, "auth failed")
				return
			}

			q.newConn(conn, false, c.RemoteAddr().String(), peerID)

			fmt.Printf("Peer %s authenticated and TLS confirmed\n", peerID)

		}(conn)

	}
}

func (q *QuicTransport) ConnChan() <-chan NewConnEvent {
	return q.connChan
}

func (q *QuicTransport) newConn(conn *quic.Conn, isOut bool, addr string, PeerID []byte) {
	q.connChan <- NewConnEvent{
		Conn:   conn,
		IsOut:  isOut,
		PeerID: PeerID,
		Addr:   addr,
	}
}

func (q *QuicTransport) authenticatePeer(stream *quic.Stream) ([]byte, error) {

	myNonce := make([]byte, 32)
	rand.Read(myNonce)

	if err := sendHandshake(stream, myNonce); err != nil {
		return nil, err
	}

	if err := acceptHandshake(stream, q.privKey); err != nil {
		return nil, err
	}

	peerPubKey, err := checkHandshakeResponse(stream, myNonce)
	if err != nil {
		return nil, err
	}

	return peerPubKey, nil
}

type PeerWrapper struct {
	conn         *quic.Conn
	hashedPeerID string

	ctx    context.Context
	cancel context.CancelFunc

	onData func(msgType MessageType, payload []byte, peerID string)
}

func NewPeerWrapper(conn *quic.Conn, parentCtx context.Context) *PeerWrapper {
	ctx, cancel := context.WithCancel(parentCtx)
	return &PeerWrapper{
		conn:   conn,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (p *PeerWrapper) Close() error {
	p.cancel()
	return p.conn.CloseWithError(ErrCodeNormalClose, "normal close")
}

func (p *PeerWrapper) OnData(onData func(msgType MessageType, payload []byte, peerID string)) {
	p.onData = onData
}

func (p *PeerWrapper) GetConn() *quic.Conn {
	return p.conn
}

func (p *PeerWrapper) RemoteAddr() net.Addr {
	return p.conn.RemoteAddr()
}

func (p *PeerWrapper) StartLoops() {
	go p.AcceptLoop(p.ctx)

	go p.AcceptDatagramLoop(p.ctx)
}

func (p *PeerWrapper) SendGossipMessage(msgType MessageType, data []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := p.conn.OpenUniStreamSync(ctx)
	if err != nil {
		return err
	}

	stream.SetWriteDeadline(time.Now().Add(1 * time.Second))

	if err := writeFrame(stream, msgType, data); err != nil {
		return err
	}

	return stream.Close()
}

func (p *PeerWrapper) SendGossipDatagram(data []byte) error {
	if len(data) > 1200 {
		return fmt.Errorf("message too big for datagram")
	}
	return p.conn.SendDatagram(data)
}

func (p *PeerWrapper) AcceptLoop(ctx context.Context) {
	for {
		select {
		case <-p.ctx.Done():
			return
		default:
		}
		stream, err := p.conn.AcceptUniStream(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Println("Peer disconnected: ", err)
			return
		}

		go p.handleGossipStream(stream)
	}
}

func (p *PeerWrapper) AcceptDatagramLoop(ctx context.Context) {
	for {
		select {
		case <-p.ctx.Done():
			return
		default:
		}
		msg, err := p.conn.ReceiveDatagram(ctx)
		if err != nil {
			return
		}

		fmt.Printf("Gossip (Unreliable): %s\n", string(msg))
	}
}

func (p *PeerWrapper) handleGossipStream(stream *quic.ReceiveStream) {
	msgType, msg, err := readFrame(stream)
	if err == nil && p.onData != nil {
		p.onData(msgType, msg, p.hashedPeerID)
	}
}

func (p *PeerWrapper) OpenStream(ctx context.Context) (*quic.Stream, error) {
	return p.conn.OpenStreamSync(ctx)
}
