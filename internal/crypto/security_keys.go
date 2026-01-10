package crypto

import (
	"P2PMessenger/internal/storage"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/dgraph-io/badger/v4"
	jsoniter "github.com/json-iterator/go"
)

const ConfigPath = "node_identity.json"

type NodeKeys struct {
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
}

func GetPublicKey(privKey ed25519.PrivateKey) ed25519.PublicKey {
	pubItf := privKey.Public()
	pub := pubItf.(ed25519.PublicKey)
	return pub
}

// GetOrGenerateKeys loads keys from a file or creates new ones
func GetOrGenerateKeys(s storage.Storage) (ed25519.PrivateKey, error) {
	var (
		keys NodeKeys
		json = jsoniter.ConfigCompatibleWithStandardLibrary
	)

	keysBytes, err := s.Get([]byte("secret:keys"))
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			pub, priv, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				return nil, err
			}

			keys.PublicKey, keys.PrivateKey = hex.EncodeToString(pub), hex.EncodeToString(priv)

			keysBytes, err = json.Marshal(&keys)
			if err != nil {
				return nil, err
			}

			err = s.Set([]byte("secret:keys"), keysBytes)
			if err != nil {
				return nil, err
			}

			return priv, nil
		} else {
			log.Println(err)
			return nil, err
		}
	}

	err = json.Unmarshal(keysBytes, &keys)
	if err != nil {
		return nil, err
	}

	if err == nil && keys.PrivateKey != "" && keys.PublicKey != "" {
		return decodeKey(keys.PrivateKey)
	} else {
		return nil, err
	}

}

// GenerateTLSConfig creates a config for QUIC/TLS based on the Identity key
func GenerateTLSConfig(priv ed25519.PrivateKey) (*tls.Config, error) {
	pub := priv.Public().(ed25519.PublicKey)
	pubHex := hex.EncodeToString(pub)

	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName:   pubHex,
			Organization: []string{"P2P-Gossip-Messenger"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour * 24 * 365 * 10),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, pub, priv)
	if err != nil {
		return nil, err
	}

	privBytes, _ := x509.MarshalPKCS8PrivateKey(priv)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates:       []tls.Certificate{tlsCert},
		NextProtos:         []string{"my-gossip-protocol"},
		ClientAuth:         tls.RequestClientCert,
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			cert, _ := x509.ParseCertificate(rawCerts[0])

			_ = cert
			// TODO run a certificate check

			return nil
		},
		ClientSessionCache: tls.NewLRUClientSessionCache(100),
	}, nil
}

func loadConfig() (*NodeKeys, error) {
	data, err := os.ReadFile(ConfigPath)
	if err != nil {
		return nil, err
	}
	var keys NodeKeys
	if err := json.Unmarshal(data, &keys); err != nil {
		return nil, err
	}
	return &keys, nil
}

func saveConfig(pub, priv string) error {
	keys := NodeKeys{PublicKey: pub, PrivateKey: priv}
	data, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigPath, data, 0600)
}

func decodeKey(hexStr string) (ed25519.PrivateKey, error) {
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, err
	}
	if len(data) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid private key size")
	}
	return ed25519.PrivateKey(data), nil
}

func GetPeerID(privKey ed25519.PrivateKey) string {
	pubKey := privKey.Public()

	edPub := pubKey.(ed25519.PublicKey)

	return hex.EncodeToString(edPub)
}
