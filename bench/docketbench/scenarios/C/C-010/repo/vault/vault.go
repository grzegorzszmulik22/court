package vault

// Secret is one stored value.
type Secret struct {
	Key   string
	Value []byte
}

// Store keeps secrets in memory.
type Store struct{ m map[string]Secret }

// New makes an empty store.
func New() *Store { return &Store{m: map[string]Secret{}} }

// Set stores a secret.
func (s *Store) Set(k string, v []byte) { s.m[k] = Secret{Key: k, Value: v} }

// Has reports presence.
func (s *Store) Has(k string) bool { _, ok := s.m[k]; return ok }
