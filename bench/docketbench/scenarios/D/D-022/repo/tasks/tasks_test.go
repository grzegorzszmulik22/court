package tasks

import "testing"

func TestAddOpen(t *testing.T) {
	var l List
	l.Add("write spec")
	l.Add("ship it")
	if got := l.Open(); got != 2 {
		t.Fatalf("Open = %d, want 2", got)
	}
}

func TestDone(t *testing.T) {
	var l List
	id := l.Add("x")
	l.items[0].Done = true
	_ = id
	if got := l.Open(); got != 0 {
		t.Fatalf("Open = %d, want 0", got)
	}
}
