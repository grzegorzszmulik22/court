package chat

import "testing"

func TestSay(t *testing.T) {
	var r Room
	r.Say("ada", "hi")
	r.Say("bo", "yo")
	if r.Count() != 2 {
		t.Fatalf("Count = %d", r.Count())
	}
}

func TestSeq(t *testing.T) {
	var r Room
	r.Say("a", "1")
	r.Say("a", "2")
	if r.msgs[1].Seq != 2 {
		t.Fatalf("Seq = %d", r.msgs[1].Seq)
	}
}
