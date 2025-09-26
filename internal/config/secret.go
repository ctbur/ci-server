package config

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// pkcs5Unpad removes the PKCS#5 padding from a byte slice.
// This is necessary because AES-CBC operates on blocks, and the final block
// is padded to the correct size before encryption.
func pkcs5Unpad(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, fmt.Errorf("data is empty")
	}
	padding := int(data[length-1])
	if padding > length {
		return nil, fmt.Errorf("padding is invalid")
	}
	return data[:(length - padding)], nil
}

// decryptSecret takes a base64-encoded encrypted string and a hexadecimal key,
// and returns the decrypted plaintext or an error.
func decryptSecret(secretKeyHex string, encryptedSecret string) (string, error) {
	// Separate IV from cyphertext
	if len(encryptedSecret) <= 32 {
		return "", fmt.Errorf("encrypted secret is too short to contain IV and ciphertext")
	}
	ivHex := encryptedSecret[:32]
	ciphertextB64 := encryptedSecret[32:]

	// Decode ciphertext, IV, and key
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext from base64: %w", err)
	}
	iv, err := hex.DecodeString(ivHex)
	if err != nil {
		return "", fmt.Errorf("failed to decode IV from hex: %w", err)
	}
	key, err := hex.DecodeString(secretKeyHex)
	if err != nil {
		return "", fmt.Errorf("failed to decode key from hex: %w", err)
	}

	// Decrpypt the data using AES-CBC
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}
	decrypter := cipher.NewCBCDecrypter(block, iv)

	decryptedBytes := make([]byte, len(ciphertext))
	copy(decryptedBytes, ciphertext)
	decrypter.CryptBlocks(decryptedBytes, decryptedBytes)

	// Remove PKCS#5 padding
	unpaddedBytes, err := pkcs5Unpad(decryptedBytes)
	if err != nil {
		return "", fmt.Errorf("failed to unpad decrypted data: %w", err)
	}

	return string(unpaddedBytes), nil
}
