//go:build !windows

package saved

func encryptPayload(plaintext []byte) ([]byte, error) {
	return plaintext, nil
}

func decryptPayload(ciphertext []byte) ([]byte, error) {
	return ciphertext, nil
}
