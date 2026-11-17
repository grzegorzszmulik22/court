package files

import "testing"

func TestSize(t *testing.T) {
	var i Index
	i.Add(Record{"a.txt", 10})
	i.Add(Record{"b.txt", 20})
	if i.Size() != 30 {
		t.Fatalf("Size = %d", i.Size())
	}
}

func TestEmpty(t *testing.T) {
	var i Index
	if i.Size() != 0 {
		t.Fatal("empty index nonzero")
	}
}
