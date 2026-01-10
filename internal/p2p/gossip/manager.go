package gossip

import (
	"P2PMessenger/internal/network"
	"P2PMessenger/internal/p2p"
	internal_pb "P2PMessenger/internal/proto"
	"bytes"
	"encoding/hex"
	"sync"
)

type Manager struct {
	swarm *p2p.Swarm

	mu        sync.RWMutex
	seenCache map[string]bool
}

func NewManager(swarm *p2p.Swarm) *Manager {
	return &Manager{
		swarm:     swarm,
		seenCache: make(map[string]bool),
	}
}

func (g *Manager) Broadcast(msgType network.MessageType, msgData *internal_pb.MessageData) {
	g.mu.Lock()
	if g.seenCache[msgData.MessageId] {
		g.mu.Unlock()
		return
	}
	g.seenCache[msgData.MessageId] = true
	g.mu.Unlock()

	msgData.HopLimit--
	if msgData.HopLimit <= 0 {
		return
	}

	peers := g.swarm.GetAllPeers()

	for _, peer := range peers {

		if bytes.Equal(peer.PubKey(), msgData.OriginId) {
			continue
		}

		go peer.Send(msgType, msgData)
	}
}

func (g *Manager) HandleIncoming(msgType network.MessageType, msgData *internal_pb.MessageData) {
	g.mu.RLock()
	seen := g.seenCache[hex.EncodeToString(msgData.OriginId)]
	g.mu.RUnlock()

	if seen {
		return
	}

	g.Broadcast(msgType, msgData)
}
