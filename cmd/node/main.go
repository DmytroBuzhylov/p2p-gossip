package main

import (
	"P2PMessenger/internal/config"
	"P2PMessenger/internal/crypto"
	"P2PMessenger/internal/dispatcher"
	"P2PMessenger/internal/network"
	"P2PMessenger/internal/p2p"
	"P2PMessenger/internal/p2p/gossip"
	"P2PMessenger/internal/services/discovery"
	"P2PMessenger/internal/services/messenger"
	"P2PMessenger/internal/storage"
	"context"
	"fmt"
	"log"

	"github.com/dgraph-io/badger/v4"
)

func main() {
	appCfg := config.GetCFG()

	opts := badger.DefaultOptions("data")
	db, err := badger.Open(opts)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	badgerStorage := storage.NewBadgerStorage(db)

	keys, err := crypto.GetOrGenerateKeys(badgerStorage)
	if err != nil {
		fmt.Println(err)
		return
	}

	tlsConfig, err := crypto.GenerateTLSConfig(keys)
	if err != nil {
		fmt.Println(err)
		return
	}

	disp := dispatcher.NewDispatcher()
	disp.Start()
	quicTransport := network.NewQUICTransport(":4242", tlsConfig, network.GetQuicConfig(), keys)

	sw := p2p.NewSwarm(crypto.GetPeerID(keys), disp, quicTransport.ConnChan(), badgerStorage, appCfg)
	quicTransport.ListenEarly(context.Background())

	eng, err := crypto.NewEngine(keys)
	if err != nil {
		log.Println(err)
		return
	}
	gsp := gossip.NewManager(sw)

	discService := discovery.NewDiscoveryService(badgerStorage, gsp, sw, crypto.GetPublicKey(keys))
	mesService := messenger.NewMessageService(keys, eng, badgerStorage, gsp)
	disp.Registry("ChatMessage", mesService)
	disp.Registry("DiscoveryPeer", discService)

	_ = sw

	fmt.Scanln()
}
