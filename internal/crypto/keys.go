// Package crypto generates and stores the node's ActivityPub key pair.
// The private key never leaves the local keys directory.
package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
)

const (
	PrivateFile = "actor.pem"
	PublicFile  = "actor.pub.pem"
	rsaBits     = 2048
)

// KeyStore persists an RSA key pair as PEM files. A later OS-keychain
// implementation can replace this type without changing callers.
type KeyStore struct {
	Dir string
}

func (s KeyStore) PrivatePath() string {
	return filepath.Join(s.Dir, PrivateFile)
}

func (s KeyStore) PublicPath() string {
	return filepath.Join(s.Dir, PublicFile)
}

func (s KeyStore) Exists() bool {
	_, err := os.Stat(s.PrivatePath())
	return err == nil
}

// LoadOrCreate generates an RSA-2048 key pair on first use.
func (s KeyStore) LoadOrCreate() error {
	if s.Dir == "" {
		return fmt.Errorf("keys directory is empty")
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("create keys dir: %w", err)
	}
	if s.Exists() {
		if _, err := s.PublicPEM(); err != nil {
			return err
		}
		return nil
	}
	priv, err := rsa.GenerateKey(rand.Reader, rsaBits)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	privPEM, err := marshalPrivate(priv)
	if err != nil {
		return err
	}
	pubPEM, err := marshalPublic(&priv.PublicKey)
	if err != nil {
		return err
	}
	if err := writeFile(s.PrivatePath(), privPEM, 0o600); err != nil {
		return err
	}
	if err := writeFile(s.PublicPath(), pubPEM, 0o600); err != nil {
		return err
	}
	return nil
}

func (s KeyStore) PublicPEM() ([]byte, error) {
	data, err := os.ReadFile(s.PublicPath())
	if err != nil {
		return nil, fmt.Errorf("read public key: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("public key file is not a PEM PUBLIC KEY")
	}
	return data, nil
}

// PublicKeyPEM returns the PKIX public key as a PEM string for Actor documents.
func (s KeyStore) PublicKeyPEM() (string, error) {
	data, err := s.PublicPEM()
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s KeyStore) PublicKey() (*rsa.PublicKey, error) {
	data, err := s.PublicPEM()
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("public key file is not a PEM PUBLIC KEY")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not RSA")
	}
	return key, nil
}

func (s KeyStore) PrivateKey() (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(s.PrivatePath())
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("private key file is not a PEM PRIVATE KEY")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not RSA")
	}
	return key, nil
}

// ParsePublicKeyPEM accepts PKIX PUBLIC KEY and PKCS#1 RSA PUBLIC KEY.
func ParsePublicKeyPEM(data []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("public key is not PEM")
	}
	switch block.Type {
	case "PUBLIC KEY":
		parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse public key: %w", err)
		}
		key, ok := parsed.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("public key is not RSA")
		}
		return key, nil
	case "RSA PUBLIC KEY":
		key, err := x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse RSA public key: %w", err)
		}
		return key, nil
	default:
		return nil, fmt.Errorf("unsupported public key type %q", block.Type)
	}
}

func marshalPrivate(key *rsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

func marshalPublic(key *rsa.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal public key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
}

func writeFile(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace %s: %w", filepath.Base(path), err)
	}
	_ = os.Chmod(path, mode)
	return nil
}
