package messenger

import (
	"P2PMessenger/internal/crypto"
	"P2PMessenger/internal/network"
	"P2PMessenger/internal/p2p/gossip"
	internal_pb "P2PMessenger/internal/proto"
	"P2PMessenger/internal/storage"
	"crypto/ed25519"
	"log"
	"time"

	"github.com/google/uuid"
)

type MessageService struct {
	cryptoEngine *crypto.Engine
	storage      storage.Storage
	gsp          *gossip.Manager
	myPrivKey    ed25519.PrivateKey
}

func NewMessageService(myPrivKey ed25519.PrivateKey, cryptoEngine *crypto.Engine, storage storage.Storage, gsp *gossip.Manager) *MessageService {
	return &MessageService{
		cryptoEngine: cryptoEngine,
		storage:      storage,
		gsp:          gsp,
		myPrivKey:    myPrivKey,
	}
}

func (s *MessageService) Handle(msg *internal_pb.MessageData, peerID string) {
	encryptedPayload := msg.Payload.(*internal_pb.MessageData_ChatMessage).ChatMessage.GetEncryptedPayload()

	data, err := s.cryptoEngine.Decrypt(encryptedPayload, msg.GetOriginId())
	if err != nil {
		log.Println(err)
		return
	}

	log.Println("Decrypted data: ", string(data))
}

func (s *MessageService) Send(toPeerID []byte, data []byte) error {
	encryptData, err := s.cryptoEngine.Encrypt(data, toPeerID)
	if err != nil {
		return err
	}

	chatMessage := s.PackChatMessage(encryptData, crypto.GetPublicKey(s.myPrivKey), toPeerID)
	s.gsp.Broadcast(network.TypeChatMessage, chatMessage)
	return nil
}

func (s *MessageService) PackChatMessage(encryptedPayload []byte, from []byte, to []byte) *internal_pb.MessageData {
	return &internal_pb.MessageData{
		MessageId: uuid.NewString(),
		OriginId:  from,
		TargetId:  to,
		Timestamp: uint64(time.Now().UnixNano()),
		HopLimit:  20,
		Payload: &internal_pb.MessageData_ChatMessage{
			ChatMessage: &internal_pb.ChatMessage{
				EncryptedPayload: encryptedPayload,
			},
		},
	}
}
