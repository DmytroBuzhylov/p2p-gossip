package dag

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"

	"github.com/DmytroBuzhylov/echofog-core/internal/crypto"
	"github.com/DmytroBuzhylov/echofog-core/internal/storage"
	"google.golang.org/protobuf/proto"
)

const ChunkSize = 1024 * 256

type NodeType int

const MaxLinksPerNode = 174

const (
	TypeFile NodeType = iota
	TypeDir
	TypeMetadata
	TypeSymlink
	TypeCommit
	TypeRaw
)

type ProNode struct {
	Type       NodeType
	Data       []byte
	Links      []Link
	Size       uint64
	Blocksizes []uint64
}

func (p *ProNode) toProto() (PBNode, error) {
	var nodeType PBNode_DataType
	switch p.Type {
	case TypeFile:
		nodeType = PBNode_File
	case TypeDir:
		nodeType = PBNode_Directory
	case TypeCommit:
		nodeType = PBNode_Commit
	case TypeMetadata:
		nodeType = PBNode_Metadata
	case TypeSymlink:
		nodeType = PBNode_Symlink
	case TypeRaw:
		nodeType = PBNode_Raw
	default:
		return PBNode{}, errors.New("invalid ProNode type")
	}

	if p.Links == nil {
		return PBNode{
			Type:       nodeType,
			Data:       p.Data,
			Links:      make([]*PBLink, 0),
			Filesize:   p.Size,
			Blocksizes: p.Blocksizes,
		}, nil
	}

	pbLinks := make([]*PBLink, 0, len(p.Links))
	for _, l := range p.Links {
		pbL, err := l.toProto()
		if err != nil {
			return PBNode{}, err
		}
		pbLinks = append(pbLinks, &pbL)
	}

	return PBNode{
		Type:       nodeType,
		Data:       p.Data,
		Links:      pbLinks,
		Filesize:   p.Size,
		Blocksizes: p.Blocksizes,
	}, nil
}

func (p *ProNode) ToProtoBytes() ([]byte, error) {
	pbNode, err := p.toProto()
	if err != nil {
		return nil, err
	}
	return proto.Marshal(&pbNode)
}

func (p *ProNode) GetHash() (string, error) {
	protoBytes, err := p.ToProtoBytes()
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(protoBytes)
	return hex.EncodeToString(hash[:]), nil
}

type Link struct {
	Hash  string
	Name  string
	TSize uint64
}

func (l *Link) toProto() (PBLink, error) {
	hash, err := hex.DecodeString(l.Hash)
	if err != nil {
		return PBLink{}, err
	}
	return PBLink{
		Hash:  hash,
		Name:  l.Name,
		Tsize: l.TSize,
	}, nil

}

type DAGBuilder struct {
	kvStorage  storage.Storage
	medService storage.MediaService
}

func NewDAGBuilder() *DAGBuilder {
	return &DAGBuilder{}
}

func (b *DAGBuilder) CreateDAG(reader io.Reader) (string, error) {
	bufPtr := chunkBufferPool.Get().([]byte)

	defer chunkBufferPool.Put(bufPtr)

	buf := bufPtr

	var links []Link

	for {
		n, err := io.ReadFull(reader, buf)
		if n > 0 {
			hash := crypto.CalculateHashHex(buf[:n])

			if err := b.medService.StoreBlock(hash, buf[:n]); err != nil {
				return "", err
			}

			links = append(links, Link{
				Hash:  hash,
				Name:  "",
				TSize: uint64(len(buf[:n])),
			})
		}

		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return "", err
		}
	}

	return b.buildTreeRecursive(links, TypeFile)
}

func (b *DAGBuilder) buildTreeRecursive(links []Link, Type NodeType) (string, error) {
	if len(links) == 1 {
		return links[0].Hash, nil
	}

	if len(links) <= MaxLinksPerNode {
		return b.createProNode(links, nil, Type)
	}

	var parentLinks []Link

	for i := 0; i < len(links); i += MaxLinksPerNode {
		end := i + MaxLinksPerNode
		if end > len(links) {
			end = len(links)
		}

		group := links[i:end]

		rootHash, err := b.createProNode(group, nil, Type)
		if err != nil {
			return "", err
		}

		var totalSize uint64
		for _, l := range group {
			totalSize += l.TSize
		}

		parentLinks = append(parentLinks, Link{
			Hash:  rootHash,
			TSize: totalSize,
		})
	}

	return b.buildTreeRecursive(parentLinks, Type)
}

func (b *DAGBuilder) createProNode(links []Link, data []byte, Type NodeType) (string, error) {
	var totalSize uint64
	for _, l := range links {
		totalSize += l.TSize
	}
	node := ProNode{
		Type:  Type,
		Data:  data,
		Links: links,
		Size:  totalSize,
	}

	blocksizes := make([]uint64, 0, len(links))
	for _, l := range links {
		blocksizes = append(blocksizes, l.TSize)
	}
	node.Blocksizes = blocksizes

	protoBytes, err := node.ToProtoBytes()
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(protoBytes)

	key := append([]byte("dag_node:"), hash[:]...)
	err = b.kvStorage.Set(key, protoBytes)
	if err != nil {
		return "", err
	}
	hexHash := hex.EncodeToString(hash[:])

	return hexHash, nil
}

func (b *DAGBuilder) storeChunk(data []byte) (string, error) {
	hash := crypto.CalculateHashHex(data)

	err := b.medService.StoreBlock(hash, data)
	return hash, err
}

func (b *DAGBuilder) storeNode(node *ProNode) (string, error) {
	data, _ := node.ToProtoBytes()
	hash := crypto.CalculateHashHex(data)

	key := []byte("dag_node:" + hash)
	err := b.kvStorage.Set(key, data)

	return hash, err
}

type DagReader struct {
	rootNode      ProNode
	rootHash      string
	medService    storage.MediaService
	kvStorage     storage.Storage
	offset        int64
	size          int64
	currentReader io.Reader
}

func NewDagReader(root ProNode, store storage.MediaService, kvStorage storage.Storage) *DagReader {
	rootHash, _ := root.GetHash()
	return &DagReader{
		rootNode:   root,
		medService: store,
		offset:     0,
		size:       int64(root.Size),
		kvStorage:  kvStorage,
		rootHash:   rootHash,
	}
}

func (d *DagReader) getNode(hexHash string) (*ProNode, error) {
	hash, err := hex.DecodeString(hexHash)
	if err != nil {
		return nil, err
	}

	key := append([]byte("dag_node:"), hash...)
	data, err := d.kvStorage.Get(key)
	if err != nil {
		return nil, err
	}
	var proNodePb PBNode
	err = proto.Unmarshal(data, &proNodePb)
	if err != nil {
		return nil, err
	}

	proNode, err := pbNodeToProNode(&proNodePb)
	if err != nil {
		return nil, err
	}
	return &proNode, nil
}

func (d *DagReader) Read(p []byte) (n int, err error) {
	if d.offset >= d.size {
		return 0, io.EOF
	}

	for n < len(p) {
		if d.currentReader == nil {
			if d.offset >= d.size {
				return n, io.EOF
			}

			reader, err := d.readNode(d.rootHash, uint64(d.offset))
			if err != nil {
				return n, err
			}
			d.currentReader = reader
		}

		readNow, readErr := d.currentReader.Read(p[n:])

		n += readNow
		d.offset += int64(readNow)

		if readErr == io.EOF {
			d.currentReader = nil
			continue
		}

		if readErr != nil {
			return n, readErr
		}
	}

	return n, nil
}

func (d *DagReader) readNode(hashHex string, offset uint64) (io.Reader, error) {

	proNode, err := d.getNode(hashHex)
	if err != nil {
		fileReader, fileErr := d.medService.ReadBlock(hashHex)
		if fileErr != nil {
			return nil, fileErr
		}
		if offset > 0 {
			_, err = fileReader.Seek(int64(offset), io.SeekStart)
			if err != nil {
				return nil, err
			}
		}
		return fileReader, nil
	}

	var currentPos uint64 = 0
	for _, link := range proNode.Links {

		if offset < (currentPos + link.TSize) {

			newOffset := offset - currentPos

			return d.readNode(link.Hash, newOffset)
		}
		currentPos += link.TSize
	}
	return nil, errors.New("offset out of range for this node")
}

func (d *DagReader) Seek(offset int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = d.offset + offset
	case io.SeekEnd:
		abs = d.size + offset
	default:
		return 0, errors.New("invalid whence")
	}
	if abs < 0 {
		return 0, errors.New("negative position")
	}
	if abs > d.size {
		return 0, errors.New("offset biggest total size")
	}

	if abs != d.offset {
		d.currentReader = nil
	}
	d.offset = abs
	return abs, nil
}
