package dispatcher

import (
	"P2PMessenger/internal/crypto"
	internal_pb "P2PMessenger/internal/proto"
	"crypto/ed25519"
	"log"
	"sync"

	"google.golang.org/protobuf/proto"
)

type Handler interface {
	Handle(msg *internal_pb.MessageData, peerID string)
}

type Dispatcher struct {
	// The channel on which ALL peers send incoming packets
	ingressChan chan IngressPacket

	handlersMu sync.RWMutex
	handlers   map[string]Handler // string = payload type
}

type IngressPacket struct {
	Envelope       *internal_pb.Envelope
	FromPeerHashID string
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		ingressChan: make(chan IngressPacket, 1000),
		handlers:    make(map[string]Handler),
	}
}

func (d *Dispatcher) Registry(payloadType string, handler Handler) {
	d.handlersMu.Lock()
	defer d.handlersMu.Unlock()
	d.handlers[payloadType] = handler
}

// PushMessage calls Peer when it has read something from the network
func (d *Dispatcher) PushMessage(env *internal_pb.Envelope, peerID string) {
	d.ingressChan <- IngressPacket{
		Envelope:       env,
		FromPeerHashID: peerID,
	}
}

func (d *Dispatcher) Start() {
	go func() {
		for packet := range d.ingressChan {
			d.processEnvelope(packet.Envelope, packet.FromPeerHashID)
		}
	}()
}

func (d *Dispatcher) processEnvelope(env *internal_pb.Envelope, fromPeer string) {
	pubKey := ed25519.PublicKey(env.PubKey)
	if len(pubKey) != 32 || !crypto.VerifySignature(pubKey, env.Data, env.Signature) {
		log.Printf("Security Alert: Invalid signature from peer!")
		return
	}

	var msgData internal_pb.MessageData
	if err := proto.Unmarshal(env.Data, &msgData); err != nil {
		log.Printf("Failed to unmarshal MessageData: %v", err)
		return
	}

	d.route(&msgData, fromPeer)
}

func (d *Dispatcher) route(msg *internal_pb.MessageData, fromPeer string) {

	var payloadType string

	switch msg.Payload.(type) {
	case *internal_pb.MessageData_ChatMessage:
		payloadType = "ChatMessage"
	case *internal_pb.MessageData_HandshakeResponse:
		payloadType = "HandshakeResponse"
	case *internal_pb.MessageData_PeerReq:
		payloadType = "DiscoveryPeer"
	case *internal_pb.MessageData_PeerRes:
		payloadType = "DiscoveryPeer"
	default:
		log.Printf("Unknown payload type from %s", fromPeer)
		return
	}

	d.handlersMu.RLock()
	handler, ok := d.handlers[payloadType]
	d.handlersMu.RUnlock()

	if ok {
		handler.Handle(msg, fromPeer)
	}
}
