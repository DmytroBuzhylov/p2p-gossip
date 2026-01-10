package network

import (
	"P2PMessenger/internal/crypto"
	internal_pb "P2PMessenger/internal/proto"
	"crypto/ed25519"
	"errors"

	"github.com/quic-go/quic-go"
	"google.golang.org/protobuf/proto"
)

func sendHandshake(stream *quic.Stream, nonce []byte) error {
	//nonce := make([]byte, 32)
	//rand.Read(nonce)

	hs := &internal_pb.MessageData{
		Payload: &internal_pb.MessageData_HandshakeInit{
			HandshakeInit: &internal_pb.HandshakeInit{Nonce: nonce},
		},
	}
	data, err := proto.Marshal(hs)
	if err != nil {
		return err
	}

	return writeFrame(stream, TypeHandshake, data)
}

func checkHandshakeResponse(stream *quic.Stream, nonce []byte) ([]byte, error) {

	msgType, protoData, err := readFrame(stream)
	if err != nil {
		return nil, err
	}
	if msgType != TypeHandshake {
		return nil, errors.New("message type is not handshake")
	}

	var data internal_pb.MessageData
	err = proto.Unmarshal(protoData, &data)
	if err != nil {
		return nil, err
	}

	switch msg := data.Payload.(type) {
	case *internal_pb.MessageData_HandshakeResponse:
		dataForVerify := append(nonce, msg.HandshakeResponse.GetPubKey()...)

		ok := crypto.VerifySignature(msg.HandshakeResponse.GetPubKey(), dataForVerify, msg.HandshakeResponse.GetSignature())
		if !ok {
			return nil, errors.New("invalid signature")
		}

		return msg.HandshakeResponse.GetPubKey(), nil
	default:
		return nil, errors.New("invalid proto type")
	}

}

func acceptHandshake(stream *quic.Stream, privKey ed25519.PrivateKey) error {
	msgType, protoData, err := readFrame(stream)
	if err != nil {
		return err
	}
	if msgType != TypeHandshake {
		return errors.New("message type is not handshake")
	}

	var data internal_pb.MessageData
	err = proto.Unmarshal(protoData, &data)
	if err != nil {
		return err
	}
	hs, ok := data.Payload.(*internal_pb.MessageData_HandshakeInit)
	if !ok {
		return errors.New("invalid proto type")
	}

	dataForSiganature := append(hs.HandshakeInit.GetNonce(), crypto.GetPublicKey(privKey)...)

	signature := crypto.CreateSignature(dataForSiganature, privKey)

	hsResp := &internal_pb.MessageData{
		Payload: &internal_pb.MessageData_HandshakeResponse{
			HandshakeResponse: &internal_pb.HandshakeResponse{
				PubKey:    crypto.GetPublicKey(privKey),
				Signature: signature,
				Version:   "1-0-0",
			},
		},
	}

	bytes, err := proto.Marshal(hsResp)
	if err != nil {
		return err
	}
	return writeFrame(stream, TypeHandshake, bytes)
}
