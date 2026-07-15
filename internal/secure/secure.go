package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

const KeySize = 32

var encodings = []*base64.Encoding{
	base64.RawURLEncoding,
	base64.URLEncoding,
	base64.RawStdEncoding,
	base64.StdEncoding,
}

func GenerateKey() (string, error) {
	key := make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(key), nil
}

func GenerateToken(bytes int) (string, error) {
	if bytes < 16 {
		return "", errors.New("token length must be at least 16 bytes")
	}
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func ParseKey(value string) ([]byte, error) {
	for _, encoding := range encodings {
		decoded, err := encoding.DecodeString(value)
		if err == nil {
			if len(decoded) != KeySize {
				return nil, fmt.Errorf("sync key must decode to %d bytes", KeySize)
			}
			return decoded, nil
		}
	}
	return nil, errors.New("sync key must be base64 encoded")
}

func Encrypt(key []byte, aad, plaintext []byte) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, plaintext, aad)
	envelope := append(nonce, sealed...)
	return base64.RawURLEncoding.EncodeToString(envelope), nil
}

func Decrypt(key []byte, aad []byte, encoded string) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	envelope, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}
	if len(envelope) < gcm.NonceSize()+gcm.Overhead() {
		return nil, errors.New("ciphertext is too short")
	}
	nonce := envelope[:gcm.NonceSize()]
	ciphertext := envelope[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, errors.New("decrypt ciphertext: authentication failed")
	}
	return plaintext, nil
}

func EntityID(key []byte, namespace, value string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(namespace))
	mac.Write([]byte{0})
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

func EventAAD(clusterID, kind, entityID string) []byte {
	return []byte(fmt.Sprintf("codex-auth-sync/v1\x00%s\x00%s\x00%s", clusterID, kind, entityID))
}

func RandomID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("AES-256 key must be %d bytes", KeySize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
