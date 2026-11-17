package sched

import "testing"

func TestDue(t *testing.T) {
	j := Job{EverySec: 60, LastRun: 1000}
	if !Due(j, 1100) {
		t.Fatal("job should be due")
	}
	if Due(j, 1030) {
		t.Fatal("job should not be due")
	}
}

func TestNext(t *testing.T) {
	j := Job{EverySec: 60, LastRun: 1000}
	if Next(j) != 1060 {
		t.Fatalf("Next = %d", Next(j))
	}
}
