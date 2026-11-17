package vault

import "testing"

func TestSetHas(t *testing.T) {
	s := New()
	s.Set("k1", []byte("v"))
	if !s.Has("k1") {
		t.Fatal("missing key")
	}
}

func TestAbsent(t *testing.T) {
	s := New()
	if s.Has("nope") {
		t.Fatal("phantom key")
	}
}
