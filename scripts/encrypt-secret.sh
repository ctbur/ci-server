#!/bin/bash

# ==============================================================================
# OpenSSL AES GCM Bare-Key Encryption Script
# ------------------------------------------------------------------------------
# Encrypts a plaintext string using OpenSSL with AES-256-GCM and a bare key.
#
# This script uses the value from the CI_SERVER_SECRET_KEY environment variable
# directly as the encryption key. It does NOT use password-based key derivation.
#
# The CI_SERVER_SECRET_KEY MUST be a 64-character hexadecimal string (32 bytes)
# for AES-256 encryption.
#
# The output is a base64-encoded string containing the IV and the ciphertext,
# concatenated together for secure and reliable decryption.
# ==============================================================================

# Function to display usage information
usage() {
  echo "Usage: $0 <plaintext_string>"
  echo "Encrypts a string using a bare key from the environment."
  echo ""
  echo "Requires the CI_SERVER_SECRET_KEY environment variable to be set to"
  echo "a 64-character hexadecimal string (e.g., a SHA-256 hash)."
  echo ""
  echo "Example:"
  echo '  export CI_SERVER_SECRET_KEY="$(openssl rand -hex 32)"'
  echo "  ./encrypt_bare_key.sh \"This is my secret message\""
  exit 1
}

# Check for the required plaintext argument
if [ -z "$1" ]; then
  usage
fi

PLAINTEXT="$1"

# Check if the secret key environment variable is set
if [ -z "$CI_SERVER_SECRET_KEY" ]; then
  echo "Error: The CI_SERVER_SECRET_KEY environment variable is not set."
  exit 1
fi

# Validate the key length
# A 32-byte key is 64 hexadecimal characters.
if [ ${#CI_SERVER_SECRET_KEY} -ne 64 ]; then
  echo "Error: The CI_SERVER_SECRET_KEY must be a 64-character hexadecimal string for AES-256."
  exit 1
fi

# Generate a cryptographically secure random 16-byte IV (32 hex characters)
IV=$(openssl rand -hex 16)

# Perform the encryption using OpenSSL
ENCRYPTED_DATA=$(echo -n "$PLAINTEXT" | openssl enc -aes-256-cbc -e -base64 -A \
-K "$CI_SERVER_SECRET_KEY" -iv "$IV" 2>/dev/null)

if [ $? -ne 0 ]; then
  echo "Error: OpenSSL encryption failed. Check your key format."
  exit 1
fi

# Prepend the IV to the encrypted data for transport and print the value
echo "${IV}${ENCRYPTED_DATA}"
