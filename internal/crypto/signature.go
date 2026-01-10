package crypto

import (
	"crypto/ed25519"
)

func CreateSignature(payload []byte, privKey ed25519.PrivateKey) []byte {
	return ed25519.Sign(privKey, payload)
}

func VerifySignature(originID []byte, payload []byte, sig []byte) bool {
	if len(sig) == 0 {
		return false
	}

	return ed25519.Verify(ed25519.PublicKey(originID), payload, sig)
}
