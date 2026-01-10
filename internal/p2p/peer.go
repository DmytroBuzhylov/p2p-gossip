package p2p

import (
	"P2PMessenger/internal/dispatcher"
	"P2PMessenger/internal/network"
	internal_pb "P2PMessenger/internal/proto"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"sync"

	"google.golang.org/protobuf/proto"
)

type Peer struct {
	transport *network.PeerWrapper
	id        string
	pubKey    ed25519.PublicKey
	addr      string
	isOut     bool

	sendChan chan *internal_pb.Envelope
	ctx      context.Context
	cancel   context.CancelFunc

	mu      sync.RWMutex
	isReady bool

	dispatcher *dispatcher.Dispatcher
}

func NewPeer(peerID []byte, dispatcher *dispatcher.Dispatcher, addr string, isOut bool) *Peer {
	ctx, cancel := context.WithCancel(context.Background())
	hash := sha256.Sum256(peerID)
	id := hex.EncodeToString(hash[:])

	return &Peer{
		id:         id,
		pubKey:     ed25519.PublicKey(peerID),
		addr:       addr,
		isOut:      isOut,
		sendChan:   make(chan *internal_pb.Envelope, 100),
		ctx:        ctx,
		cancel:     cancel,
		dispatcher: dispatcher,
	}
}

func (p *Peer) Close() {
	p.cancel()
}

func (p *Peer) SetTransport(transport *network.PeerWrapper) {
	p.transport = transport
	p.addr = transport.RemoteAddr().String()
}

func (p *Peer) ID() string {
	return p.id
}

func (p *Peer) PubKey() []byte {
	return p.pubKey
}

func (p *Peer) Key() ed25519.PublicKey {
	return p.pubKey
}

func (p *Peer) Addr() string {
	return p.addr
}

func (p *Peer) Send(msgType network.MessageType, env *internal_pb.MessageData) error {
	data, err := proto.Marshal(env)
	if err != nil {
		log.Println(err)
		return err
	}

	return p.transport.SendGossipMessage(msgType, data)
}

func (p *Peer) readLoop() {

}

func (p *Peer) waitForResponse() {

}

func (p *Peer) handleIncomingConnection() {

}
