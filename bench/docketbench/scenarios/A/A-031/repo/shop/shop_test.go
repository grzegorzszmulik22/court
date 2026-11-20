package shop

import "testing"

func TestTotal(t *testing.T) {
	items := []Item{{"a", 100, 2}, {"b", 50, 1}}
	if got := Total(items); got != 250 {
		t.Fatalf("Total = %d, want 250", got)
	}
}

func TestDiscount(t *testing.T) {
	if got := Discount(200, 10); got != 180 {
		t.Fatalf("Discount = %d, want 180", got)
	}
}
