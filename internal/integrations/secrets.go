package integrations

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// SecretStore keeps integration tokens out of SQLite.
type SecretStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
	Delete(ctx context.Context, key string) error
}

var ErrNoMasterKey = errors.New("HS_MASTER_KEY is not set")

// FileSecrets is an AES-GCM encrypted JSON map at data/secrets.enc.
type FileSecrets struct {
	path string
	aead cipher.AEAD
	mu   sync.Mutex
	data map[string]string
}

// NewFileSecrets opens (or creates) the encrypted file. The master key may be
// 32 raw bytes as hex/base64-ish text or any passphrase (hashed with SHA-256).
func NewFileSecrets(path, masterKey string) (*FileSecrets, error) {
	if masterKey == "" {
		return nil, ErrNoMasterKey
	}
	key := make([]byte, 32)
	if b, err := hex.DecodeString(masterKey); err == nil && len(b) == 32 {
		copy(key, b)
	} else {
		sum := sha256.Sum256([]byte(masterKey))
		copy(key, sum[:])
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	f := &FileSecrets{path: path, aead: aead, data: map[string]string{}}
	if err := f.load(); err != nil {
		return nil, err
	}
	return f, nil
}

// NewMemorySecrets is for tests and for an install without HS_MASTER_KEY.
func NewMemorySecrets() *FileSecrets {
	f := &FileSecrets{data: map[string]string{}}
	return f
}

func (f *FileSecrets) load() error {
	b, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(b) < f.aead.NonceSize() {
		return errors.New("secrets: file too short")
	}
	nonce, ciphertext := b[:f.aead.NonceSize()], b[f.aead.NonceSize():]
	plain, err := f.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return errors.New("secrets: cannot decrypt (wrong HS_MASTER_KEY?)")
	}
	return json.Unmarshal(plain, &f.data)
}

func (f *FileSecrets) save() error {
	plain, err := json.Marshal(f.data)
	if err != nil {
		return err
	}
	nonce := make([]byte, f.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	out := append(nonce, f.aead.Seal(nil, nonce, plain, nil)...)
	if err := os.MkdirAll(filepath.Dir(f.path), 0o750); err != nil {
		return err
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.path)
}

func (f *FileSecrets) Get(ctx context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.data[key], nil
}

func (f *FileSecrets) Set(ctx context.Context, key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if value == "" {
		delete(f.data, key)
	} else {
		f.data[key] = value
	}
	if f.path == "" {
		return nil
	}
	return f.save()
}

func (f *FileSecrets) Delete(ctx context.Context, key string) error {
	return f.Set(ctx, key, "")
}
