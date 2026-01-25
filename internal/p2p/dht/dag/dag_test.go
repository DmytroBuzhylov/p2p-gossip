package dag

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/DmytroBuzhylov/echofog-core/internal/storage"
	"google.golang.org/protobuf/proto"
)

type MockKV struct {
	data map[string][]byte
}

func (m *MockKV) FindValues(prefix []byte) ([]interface{}, error) {
	return nil, nil
}

func (m *MockKV) Exists(key []byte) bool {
	return false
}

func NewMockKV() *MockKV {
	return &MockKV{data: make(map[string][]byte)}
}

func (m *MockKV) Set(key, val []byte) error {
	m.data[string(key)] = val
	return nil
}

func (m *MockKV) Get(key []byte) ([]byte, error) {
	val, ok := m.data[string(key)]
	if !ok {
		return nil, errors.New("key not found")
	}
	return val, nil
}

func (m *MockKV) Delete(key []byte) error { return nil }
func (m *MockKV) Close() error            { return nil }

func TestDAG_RoundTrip(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dag_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mediaService, err := storage.NewMediaService(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	os.MkdirAll(tmpDir+"/media/blocks", 0755)

	mockKV := NewMockKV()

	builder := &DAGBuilder{
		kvStorage:  mockKV,
		medService: *mediaService,
	}

	dataSize := (ChunkSize * 3) + 12345
	originalData := make([]byte, dataSize)
	_, err = rand.Read(originalData)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Generated %d bytes of random data", dataSize)

	rootHash, err := builder.CreateDAG(bytes.NewReader(originalData))
	if err != nil {
		t.Fatalf("Failed to create DAG: %v", err)
	}
	t.Logf("DAG Created! Root Hash: %s", rootHash)

	if len(mockKV.data) == 0 {
		t.Fatal("KV Storage is empty, but DAG nodes should be there")
	}

	rootNodeData, err := mockKV.Get(append([]byte("dag_node:"), decodeHex(t, rootHash)...))
	if err != nil {
		t.Fatalf("Root node not found in KV: %v", err)
	}

	var pbRoot PBNode
	if err := proto.Unmarshal(rootNodeData, &pbRoot); err != nil {
		t.Fatal(err)
	}

	proRoot, _ := pbNodeToProNode(&pbRoot)

	reader := NewDagReader(proRoot, *mediaService, mockKV)

	readBackData, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Failed to read back DAG: %v", err)
	}

	if len(readBackData) != len(originalData) {
		t.Fatalf("Size mismatch! Original: %d, Read: %d", len(originalData), len(readBackData))
	}

	if !bytes.Equal(originalData, readBackData) {
		t.Fatal("Data mismatch! The file is corrupted.")
	}

	t.Log("SUCCESS: Data read back matches original exactly.")
}

func TestDAG_Seek(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "dag_seek_*")
	defer os.RemoveAll(tmpDir)
	mediaService, _ := storage.NewMediaService(tmpDir)
	os.MkdirAll(tmpDir+"/media/blocks", 0755)
	mockKV := NewMockKV()
	builder := &DAGBuilder{kvStorage: mockKV, medService: *mediaService}

	data := make([]byte, ChunkSize*2)
	for i := 0; i < ChunkSize; i++ {
		data[i] = 'A'
	}
	for i := ChunkSize; i < ChunkSize*2; i++ {
		data[i] = 'B'
	}

	rootHash, _ := builder.CreateDAG(bytes.NewReader(data))

	rootNodeData, _ := mockKV.Get(append([]byte("dag_node:"), decodeHex(t, rootHash)...))
	var pbRoot PBNode
	proto.Unmarshal(rootNodeData, &pbRoot)
	proRoot, _ := pbNodeToProNode(&pbRoot)
	reader := NewDagReader(proRoot, *mediaService, mockKV)

	offset := int64(ChunkSize + 100)
	_, err := reader.Seek(offset, io.SeekStart)
	if err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 5)
	n, err := reader.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	if n != 5 {
		t.Fatalf("Expected 5 bytes, got %d", n)
	}

	if string(buf) != "BBBBB" {
		t.Fatalf("Seek failed! Expected 'BBBBB', got '%s'", string(buf))
	}

	t.Log("SUCCESS: Seek works correctly.")
}

func decodeHex(t *testing.T, s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
