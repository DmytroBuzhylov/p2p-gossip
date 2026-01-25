package node

import (
	"encoding/hex"

	"github.com/DmytroBuzhylov/echofog-core/pkg/api/types"
	"github.com/DmytroBuzhylov/echofog-core/pkg/dto"
	"github.com/DmytroBuzhylov/echofog-core/pkg/node"
)

type NodeApi struct {
	node *node.Node
}

func NewNodeApi(node *node.Node) *NodeApi {
	return &NodeApi{node: node}
}

func (n *NodeApi) GetPeers() []dto.PeerDTO {
	internalPeers := n.node.Swarm.GetAllPeers()

	result := make([]dto.PeerDTO, 0, len(internalPeers))

	for _, p := range internalPeers {
		id := p.ID()
		idStr := hex.EncodeToString(id[:])

		dtoPeer := dto.PeerDTO{
			PeerID:      idStr,
			ShortID:     idStr[:6],
			Address:     p.Addr(),
			IsConnected: true,
			IsLocal:     false,
		}
		result = append(result, dtoPeer)
	}

	return result

}

func (n *NodeApi) GetMyID() string {
	if n.node.ID == (types.PeerID{}) {
		return ""
	}
	return hex.EncodeToString(n.node.ID[:])
}

func (n *NodeApi) SendMessage(pubKeyHex string, text string) error {
	pubKeyBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return err
	}
	var targetPubKey types.PeerPublicKey
	copy(targetPubKey[:], pubKeyBytes)

	return n.node.Messenger.Send(targetPubKey, []byte(text))
}

func (n *NodeApi) CreateContact() {

}
