package utils

import "sync"

const MaxPacketSize = 65536

var bufPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, MaxPacketSize)

		return &b
	},
}

func GetBuffer() *[]byte {
	return bufPool.Get().(*[]byte)
}

func PutBuffer(b *[]byte) {
	bufPool.Put(b)
}
