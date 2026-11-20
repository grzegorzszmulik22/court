package poll

import "testing"

func TestTally(t *testing.T) {
	got := Tally([]Vote{{"a", 2}, {"b", 1}, {"a", 0}})
	if got["a"] != 3 || got["b"] != 1 {
		t.Fatalf("Tally = %v", got)
	}
}

func TestTallyEmpty(t *testing.T) {
	if len(Tally(nil)) != 0 {
		t.Fatal("empty tally nonzero")
	}
}
