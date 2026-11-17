package ledger

import "testing"

func TestBalance(t *testing.T) {
	var b Book
	b.Post(Entry{"cash", 100})
	b.Post(Entry{"cash", -40})
	if got := b.Balance("cash"); got != 60 {
		t.Fatalf("Balance = %d, want 60", got)
	}
}

func TestEmpty(t *testing.T) {
	var b Book
	if got := b.Balance("none"); got != 0 {
		t.Fatalf("Balance = %d, want 0", got)
	}
}
