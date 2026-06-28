package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
)

const (
	saltLen      = 16
	argonTime    = 3
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	v2Prefix     = "v2:"
)

type Store struct {
	path string
	key  []byte
	salt []byte
	mu   sync.RWMutex
	data map[string]string
}

func NewStore(path string) (*Store, error) {
	masterKey := os.Getenv("MODELMUX_MASTER_KEY")
	if masterKey == "" {
		return nil, errors.New("MODELMUX_MASTER_KEY environment variable is not set")
	}

	s := &Store{
		path: path,
		data: map[string]string{},
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err == nil {
		if err := s.decryptFile(data, masterKey); err != nil {
			return nil, fmt.Errorf("failed to decrypt secret store: %w", err)
		}
		return s, nil
	}

	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	s.salt = salt
	s.key = deriveKey(masterKey, s.salt)
	return s, nil
}

func deriveKey(masterKey string, salt []byte) []byte {
	if salt == nil || len(salt) == 0 {
		h := sha256.Sum256([]byte(masterKey))
		return h[:]
	}
	return argon2.IDKey([]byte(masterKey), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
}

func (s *Store) Get(ref string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	v, ok := s.data[ref]
	if !ok {
		return "", fmt.Errorf("secret not found: %s", ref)
	}
	return v, nil
}

func (s *Store) Set(ref, value string) error {
	s.mu.Lock()
	s.data[ref] = value
	s.mu.Unlock()
	return s.save()
}

func (s *Store) Delete(ref string) error {
	s.mu.Lock()
	delete(s.data, ref)
	s.mu.Unlock()
	return s.save()
}

func (s *Store) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var keys []string
	for k := range s.data {
		keys = append(keys, k)
	}
	return keys
}

func (s *Store) save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	plaintext, err := json.Marshal(s.data)
	if err != nil {
		return err
	}

	encrypted, err := s.encrypt(plaintext)
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, encrypted, 0o600)
}

func (s *Store) encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	encoded := base64.StdEncoding.EncodeToString(ciphertext)

	if s.salt == nil || len(s.salt) == 0 {
		return []byte(encoded), nil
	}
	return []byte(v2Prefix + hex.EncodeToString(s.salt) + ":" + encoded), nil
}

func (s *Store) decryptFile(data []byte, masterKey string) error {
	content := strings.TrimSpace(string(data))
	if strings.HasPrefix(content, v2Prefix) {
		rest := strings.TrimPrefix(content, v2Prefix)
		parts := strings.SplitN(rest, ":", 2)
		if len(parts) != 2 {
			return errors.New("invalid v2 secret store format")
		}
		salt, err := hex.DecodeString(parts[0])
		if err != nil || len(salt) != saltLen {
			return errors.New("invalid salt in v2 secret store")
		}
		s.salt = salt
		s.key = deriveKey(masterKey, s.salt)
		return s.decryptPayload([]byte(parts[1]))
	}

	legacyKey := deriveKey(masterKey, nil)
	key := legacyKey
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	decoded, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		return err
	}
	nonceSize := gcm.NonceSize()
	if len(decoded) < nonceSize {
		return errors.New("ciphertext too short")
	}
	nonce, ciphertext := decoded[:nonceSize], decoded[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(plaintext, &s.data); err != nil {
		return err
	}

	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return err
	}
	s.salt = salt
	s.key = deriveKey(masterKey, s.salt)
	return s.save()
}

func (s *Store) decryptPayload(data []byte) error {
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return err
	}

	block, err := aes.NewCipher(s.key)
	if err != nil {
		return err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	nonceSize := gcm.NonceSize()
	if len(decoded) < nonceSize {
		return errors.New("ciphertext too short")
	}

	nonce, ciphertext := decoded[:nonceSize], decoded[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return err
	}

	return json.Unmarshal(plaintext, &s.data)
}

func (s *Store) ExportData() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return os.ReadFile(s.path)
}

func ImportData(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}

func (s *Store) RotateKey(newMasterKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return err
	}
	s.salt = salt
	s.key = deriveKey(newMasterKey, s.salt)

	plaintext, err := json.Marshal(s.data)
	if err != nil {
		return err
	}
	encrypted, err := s.encrypt(plaintext)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, encrypted, 0o600)
}

func VerifyFile(path string) error {
	store, err := NewStore(path)
	if err != nil {
		return err
	}
	if len(store.data) >= 0 {
		return nil
	}
	return nil
}
