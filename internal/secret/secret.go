package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	path  string
	key   []byte
	mu    sync.RWMutex
	data  map[string]string
}

func NewStore(path string) (*Store, error) {
	masterKey := os.Getenv("MODELMUX_MASTER_KEY")
	if masterKey == "" {
		return nil, errors.New("MODELMUX_MASTER_KEY environment variable is not set")
	}

	hash := sha256.Sum256([]byte(masterKey))
	key := hash[:]

	s := &Store{
		path: path,
		key:  key,
		data: map[string]string{},
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err == nil {
		if err := s.decrypt(data); err != nil {
			return nil, fmt.Errorf("failed to decrypt secret store: %w", err)
		}
	}

	return s, nil
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
	return []byte(encoded), nil
}

func (s *Store) decrypt(data []byte) error {
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
