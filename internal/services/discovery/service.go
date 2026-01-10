package discovery

import (
	"P2PMessenger/internal/network"
	"P2PMessenger/internal/p2p"
	"P2PMessenger/internal/p2p/gossip"
	internal_pb "P2PMessenger/internal/proto"
	"P2PMessenger/internal/storage"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
)

type DiscoveryService struct {
	storage  storage.Storage
	gsp      *gossip.Manager
	swarm    *p2p.Swarm
	getter   *peerGetter
	giver    *peerGiver
	myPubKey ed25519.PublicKey
}

func NewDiscoveryService(storage storage.Storage, gsp *gossip.Manager, swarm *p2p.Swarm, myPubKey ed25519.PublicKey) *DiscoveryService {
	ds := &DiscoveryService{
		storage:  storage,
		gsp:      gsp,
		swarm:    swarm,
		myPubKey: myPubKey,
	}
	var getter = NewPeerGetter(ds)
	giver := NewPeerGiver(ds)
	ds.getter = getter
	ds.giver = giver

	return ds
}

func (d *DiscoveryService) Handle(msg *internal_pb.MessageData, peerID string) {
	switch msg.Payload.(type) {
	case *internal_pb.MessageData_PeerRes:
		d.giver.Handle(msg, peerID)
	case *internal_pb.MessageData_PeerReq:
		d.getter.Handle(msg, peerID)
	}
}

type peerGetter struct {
	service *DiscoveryService
}

func NewPeerGetter(service *DiscoveryService) *peerGetter {
	return &peerGetter{service: service}
}

func (g *peerGetter) Handle(msg *internal_pb.MessageData, peerID string) {

}

type peerGiver struct {
	service *DiscoveryService
}

func NewPeerGiver(service *DiscoveryService) *peerGiver {
	return &peerGiver{service: service}
}

func (g *peerGiver) Handle(msg *internal_pb.MessageData, peerID string) {
	msgPeer, ok := msg.Payload.(*internal_pb.MessageData_PeerReq)
	if !ok {
		return
	}

	peers := g.service.swarm.GetMyRandomPeers(uint(msgPeer.PeerReq.Count))

	peersInfo := make([]*internal_pb.PeerInfo, len(peers))
	for _, p := range peers {
		peersInfo = append(peersInfo, &internal_pb.PeerInfo{
			PubKey:  p.PubKey(),
			Address: p.Addr(),
		})
	}

	peerResponse := &internal_pb.MessageData{
		MessageId: uuid.NewString(),
		OriginId:  g.service.myPubKey,
		TargetId:  msg.GetOriginId(),
		Timestamp: uint64(time.Now().UnixNano()),
		HopLimit:  20,
		Payload: &internal_pb.MessageData_PeerRes{
			PeerRes: &internal_pb.PeerResponse{
				Peers: peersInfo,
			},
		},
	}

	hash := sha256.New()
	hashedPeerID := hex.EncodeToString(hash.Sum(msg.OriginId))

	if g.service.swarm.ThisIsActivePeer(hashedPeerID) {
		err := g.service.swarm.SendDataForPeer(hashedPeerID, network.TypeGetPeerResponse, peerResponse)
		if err == nil {
			return
		}
	}

	g.service.gsp.Broadcast(network.TypeGetPeerResponse, peerResponse)
}
