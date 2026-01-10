package p2p

import (
	"P2PMessenger/internal/config"
	"P2PMessenger/internal/dispatcher"
	"P2PMessenger/internal/network"
	internal_pb "P2PMessenger/internal/proto"
	"P2PMessenger/internal/storage"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"google.golang.org/protobuf/proto"
)

type Swarm struct {
	mu          sync.RWMutex
	activePeers map[string]*Peer

	dispatcher *dispatcher.Dispatcher
	storage    storage.Storage

	selfID       string
	netTransport network.QuicTransport

	cfg *config.AppConfig
}

func NewSwarm(selfID string, d *dispatcher.Dispatcher, connChan <-chan network.NewConnEvent, storage storage.Storage, cfg *config.AppConfig) *Swarm {
	s := &Swarm{
		activePeers: make(map[string]*Peer),
		dispatcher:  d,
		selfID:      selfID,
		storage:     storage,
		cfg:         cfg,
	}

	go s.registrationLoop(connChan)

	return s
}

func (s *Swarm) registrationLoop(ch <-chan network.NewConnEvent) {
	for event := range ch {
		p := s.AddPeer(event.PeerID, event.Conn, event.Addr, event.IsOut)

		go p.transport.StartLoops()
	}
}

func (s *Swarm) findAndConnectToPeers() {
	go func() {
		for _, peer := range s.GetHistoryConnected(0) {

			hash := sha256.Sum256(peer.PubKey)
			peerID := hex.EncodeToString(hash[:])

			s.mu.RLock()
			if len(s.activePeers) > s.cfg.MaxConnections {
				break
			}
			_, ok := s.activePeers[peerID]
			s.mu.RUnlock()
			if ok {
				continue
			}

			go s.connect(peer.GetLastKnownAddr())
		}

		s.mu.RLock()
		if len(s.activePeers) > 20 {
			return
		}
		s.mu.RUnlock()

	}()
}

func (s *Swarm) AddPeer(peerID []byte, conn *quic.Conn, addr string, isOut bool) *Peer {

	hash := sha256.Sum256(peerID)
	hashedID := hex.EncodeToString(hash[:])

	s.mu.Lock()
	if old, exists := s.activePeers[hashedID]; exists {
		old.Close()
	}
	s.mu.Unlock()

	p := NewPeer(peerID, s.dispatcher, addr, isOut)
	pw := network.NewPeerWrapper(conn, p.ctx)

	go s.SavePeer(p.pubKey, p.addr, 100)

	pw.OnData(func(msgType network.MessageType, payload []byte, peerID string) {

		var data internal_pb.Envelope
		err := proto.Unmarshal(payload, &data)
		if err != nil {
			return
		}

		s.dispatcher.PushMessage(&data, peerID)
	})

	p.SetTransport(pw)

	s.mu.Lock()
	s.activePeers[hashedID] = p
	s.mu.Unlock()

	return p
}

func (s *Swarm) RemovePeer(peerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.activePeers[peerID]; ok {
		p.Close()
		delete(s.activePeers, peerID)
	}
}

func (s *Swarm) GetPeer(id string) *Peer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activePeers[id]
}

func (s *Swarm) GetAllPeers() []*Peer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]*Peer, 0, len(s.activePeers))
	for _, p := range s.activePeers {
		res = append(res, p)
	}
	return res
}

func (s *Swarm) ThisIsActivePeer(hashedPeerID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.activePeers[hashedPeerID]
	return ok
}

func (s *Swarm) GetMyRandomPeers(count uint) []*Peer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]*Peer, 0, len(s.activePeers))

	var mapCount uint

	for _, p := range s.activePeers {
		if mapCount >= count {
			break
		}

		res = append(res, p)
		mapCount++
	}
	return res
}

func (s *Swarm) BanPeer(hashedPeerID string) {
	key := append([]byte("bans:peer:"), []byte(hashedPeerID)...)

	s.storage.Set(key, []byte("true"))

	s.mu.RLock()
	peer, ok := s.activePeers[hashedPeerID]
	s.mu.RUnlock()
	if !ok || peer == nil {
		return
	}

	peer.Close()
}

func (s *Swarm) UnBanPeer(hashedPeerID string) {
	key := append([]byte("bans:peer:"), []byte(hashedPeerID)...)
	s.storage.Delete(key)
}

func (s *Swarm) CheckOnBan(hashedPeerID string) bool {
	key := append([]byte("bans:peer:"), []byte(hashedPeerID)...)
	_, err := s.storage.Get(key)
	if err != nil {
		return true
	}
	return false
}

func (s *Swarm) SavePeer(peerPubKey []byte, addr string, trustScore uint32) {

	data := &storage.PeerStoreEntry{
		PubKey:        peerPubKey,
		LastKnownAddr: addr,
		LastSeen:      uint64(time.Now().UnixNano()),
		TrustScore:    trustScore,
	}
	protoData, err := proto.Marshal(data)
	if err != nil {
		return
	}

	hash := sha256.Sum256(peerPubKey)

	prefix := []byte("saved:peers:")
	key := make([]byte, len(prefix)+len(hash))
	copy(key, prefix)
	copy(key[len(prefix):], hash[:])

	s.storage.Set(key, protoData)
}

func (s *Swarm) connect(addr string) {
	ctx, _ := context.WithTimeout(context.Background(), time.Second*3)
	s.netTransport.DialEarly(ctx, addr)
}

func (s *Swarm) SendDataForPeer(hashedPeerID string, msgType network.MessageType, data *internal_pb.MessageData) error {
	peer := s.GetPeer(hashedPeerID)
	if peer == nil {
		return errors.New("this peer is not connected")
	}
	return peer.Send(msgType, data)
}

// GetHistoryConnected Set to 0 to get all peers
func (s *Swarm) GetHistoryConnected(count uint) []storage.PeerStoreEntry {
	findValues, err := s.storage.FindValues([]byte("saved:peers:"))
	if err != nil {
		return nil
	}
	var (
		peers     []storage.PeerStoreEntry
		peerCount uint
	)

	for _, v := range findValues {
		if count != 0 {
			if peerCount >= count {
				break
			}
		}

		var peer storage.PeerStoreEntry
		err = proto.Unmarshal(v.([]byte), &peer)
		if err != nil {
			continue
		}

		peers = append(peers, peer)

		if count != 0 {
			peerCount++
		}
	}
	return peers
}
