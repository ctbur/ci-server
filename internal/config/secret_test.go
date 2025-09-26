package config

import (
	"testing"

	"github.com/ctbur/ci-server/v2/internal/assert"
)

func TestSecretDecryption(t *testing.T) {
	secretKey := "8d6da607e4c2499088b799f4c769c77b3878fa48fb634fe459906269c70b2a59"
	secret := "1234"
	encryptedSecret := "74af5aa7fab2e0df2fbf12aa86d2b3f4W+02b8sXjN/3gjjfidZrmw=="

	decryptedSecret, err := decryptSecret(secretKey, encryptedSecret)

	assert.NoError(t, err, "Decryption failed")
	assert.Equal(t, decryptedSecret, secret, "Decrypted secret does not match plaintext")
}
