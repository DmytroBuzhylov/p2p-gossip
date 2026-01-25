package dag

import "sync"

var chunkBufferPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, ChunkSize)
	},
}
