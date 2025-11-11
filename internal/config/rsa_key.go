package config

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
)

func LoadRSAPrivateKey(reader io.Reader) (*rsa.PrivateKey, error) {
	keyData, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read key data: %w", err)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block containing private key")
	}

	var key any
	switch block.Type {
	case "PRIVATE KEY":
		// PKCS#8: Algorithm-agnostic wrapper (preferred modern format)
		key, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	case "RSA PRIVATE KEY":
		// PKCS#1: Legacy, RSA-specific format
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unsupported private key type: %s", block.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA private key")
	}

	return rsaKey, nil
}
