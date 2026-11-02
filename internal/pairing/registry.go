package pairing

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/ma99us/tau-shell/internal/fsutil"
)

// Sealer protects private keys at rest. Production code supplies a Windows
// DPAPI implementation; tests use a fake. Keys are never written unsealed by
// the registry itself.
type Sealer interface {
	Seal(plain []byte) ([]byte, error)
	Open(sealed []byte) ([]byte, error)
}

// Errors returned by Authenticate. They are deliberately coarse so a caller
// cannot learn from the message whether the id or the token was wrong.
var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrNotFound     = errors.New("no such device")
)

// Device is the public view of a paired phone. It carries no secret.
type Device struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Fingerprint string    `json:"fingerprint"`
	Created     time.Time `json:"created"`
	Expires     time.Time `json:"expires"`
	LastSeen    time.Time `json:"lastSeen,omitempty"`
}

type record struct {
	Device
	// TokenHash is the SHA-256 of the bearer token; the token itself is shown to
	// the phone once and never stored.
	TokenHash string `json:"tokenHash"`
	CertDER   []byte `json:"certDer"`
	SealedKey []byte `json:"sealedKey"`
}

// Registry persists paired devices in runtime state (not in YAML).
type Registry struct {
	mu      sync.Mutex
	path    string
	sealer  Sealer
	clock   func() time.Time
	records map[string]record
}

// OpenRegistry loads the registry at path, or starts an empty one if the file
// does not exist. A corrupt file is an error, not an empty registry, so a bad
// write can never silently un-pair or re-enable devices.
func OpenRegistry(path string, sealer Sealer, clock func() time.Time) (*Registry, error) {
	if clock == nil {
		clock = time.Now
	}
	registry := &Registry{path: path, sealer: sealer, clock: clock, records: map[string]record{}}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return registry, nil
	}
	if err != nil {
		return nil, err
	}
	var list []record
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, errors.New("device registry is unreadable: " + err.Error())
	}
	for _, item := range list {
		registry.records[item.ID] = item
	}
	return registry, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Add creates a device with its own certificate, key and token. The token is
// returned once; only its hash is kept.
func (registry *Registry) Add(name string, hosts []string, lifetime time.Duration) (Device, string, error) {
	credentials, err := New(hosts, lifetime)
	if err != nil {
		return Device{}, "", err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(credentials.Certificate.PrivateKey)
	if err != nil {
		return Device{}, "", err
	}
	sealed, err := registry.sealer.Seal(keyDER)
	if err != nil {
		return Device{}, "", err
	}
	idBytes := make([]byte, 8)
	if _, err := rand.Read(idBytes); err != nil {
		return Device{}, "", err
	}
	now := registry.clock()
	item := record{
		Device: Device{
			ID: hex.EncodeToString(idBytes), Name: name,
			Fingerprint: credentials.Fingerprint, Created: now, Expires: now.Add(lifetime),
		},
		TokenHash: hashToken(credentials.Token),
		CertDER:   credentials.Certificate.Certificate[0],
		SealedKey: sealed,
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.records[item.ID] = item
	if err := registry.saveLocked(); err != nil {
		delete(registry.records, item.ID)
		return Device{}, "", err
	}
	return item.Device, credentials.Token, nil
}

// Authenticate checks the token for a device and that it has not expired. It
// records last-seen in memory only; a request must not cause a disk write.
func (registry *Registry) Authenticate(id, token string) (Device, error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	item, found := registry.records[id]
	// Compare even when the id is unknown so timing does not reveal valid ids.
	if subtle.ConstantTimeCompare([]byte(hashToken(token)), []byte(item.TokenHash)) != 1 || !found {
		return Device{}, ErrUnauthorized
	}
	now := registry.clock()
	if now.After(item.Expires) {
		return Device{}, ErrUnauthorized
	}
	item.LastSeen = now
	registry.records[id] = item
	return item.Device, nil
}

// Certificate returns the TLS certificate for a device, or ErrNotFound for an
// unknown, revoked or expired one.
func (registry *Registry) Certificate(id string) (*tls.Certificate, error) {
	registry.mu.Lock()
	item, found := registry.records[id]
	registry.mu.Unlock()
	if !found || registry.clock().After(item.Expires) {
		return nil, ErrNotFound
	}
	keyDER, err := registry.sealer.Open(item.SealedKey)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParsePKCS8PrivateKey(keyDER)
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{Certificate: [][]byte{item.CertDER}, PrivateKey: key}, nil
}

// Revoke deletes a device and its key. Existing TLS sessions are the caller's
// to close; new handshakes and requests fail from this point.
func (registry *Registry) Revoke(id string) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	item, found := registry.records[id]
	if !found {
		return ErrNotFound
	}
	delete(registry.records, id)
	if err := registry.saveLocked(); err != nil {
		registry.records[id] = item
		return err
	}
	return nil
}

// List returns the paired devices, oldest first.
func (registry *Registry) List() []Device {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	devices := make([]Device, 0, len(registry.records))
	for _, item := range registry.records {
		devices = append(devices, item.Device)
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].Created.Before(devices[j].Created) })
	return devices
}

func (registry *Registry) saveLocked() error {
	list := make([]record, 0, len(registry.records))
	for _, item := range registry.records {
		list = append(list, item)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.ReplaceFile(registry.path, raw)
}
