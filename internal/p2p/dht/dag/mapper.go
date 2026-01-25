package dag

import (
	"encoding/hex"
	"errors"
)

func pbNodeToProNode(node *PBNode) (ProNode, error) {
	var nodeType NodeType
	switch node.Type {
	case PBNode_File:
		nodeType = TypeFile
	case PBNode_Directory:
		nodeType = TypeDir
	case PBNode_Commit:
		nodeType = TypeCommit
	case PBNode_Metadata:
		nodeType = TypeMetadata
	case PBNode_Symlink:
		nodeType = TypeSymlink
	case PBNode_Raw:
		nodeType = TypeRaw
	default:
		return ProNode{}, errors.New("invalid ProNode type")
	}

	if node.Links == nil {
		return ProNode{
			Type:       nodeType,
			Data:       node.Data,
			Links:      make([]Link, 0),
			Size:       node.Filesize,
			Blocksizes: node.Blocksizes,
		}, nil
	}

	links := make([]Link, 0, len(node.Links))
	for _, l := range node.Links {
		links = append(links, Link{
			Hash:  hex.EncodeToString(l.Hash),
			Name:  l.Name,
			TSize: l.Tsize,
		})
	}

	return ProNode{
		Type:       nodeType,
		Data:       node.Data,
		Links:      links,
		Size:       node.Filesize,
		Blocksizes: node.Blocksizes,
	}, nil
}
