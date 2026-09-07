package mailbox

import "testing"

func TestQueueDrainsFIFOWithLimit(t *testing.T) {
	q := New(4)
	got := make([]int, 0, 3)
	for i := 1; i <= 3; i++ {
		value := i
		if !q.TryPost(func() { got = append(got, value) }) {
			t.Fatalf("post %d rejected", i)
		}
	}

	if drained := q.Drain(2); drained != 2 {
		t.Fatalf("first drain = %d, want 2", drained)
	}
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("first drain order = %v, want [1 2]", got)
	}
	if q.Len() != 1 {
		t.Fatalf("queue len = %d, want 1", q.Len())
	}

	if drained := q.Drain(4); drained != 1 {
		t.Fatalf("second drain = %d, want 1", drained)
	}
	if len(got) != 3 || got[2] != 3 {
		t.Fatalf("final drain order = %v, want [1 2 3]", got)
	}
}

func TestCriticalTasksSurviveOrdinaryPressureAndRunFirst(t *testing.T) {
	q := New(1)
	got := make([]string, 0, 2)
	if !q.TryPost(func() { got = append(got, "ordinary") }) {
		t.Fatal("ordinary post rejected")
	}
	if q.TryPost(func() {}) {
		t.Fatal("ordinary queue should be full")
	}
	if !q.PostCritical(func() { got = append(got, "critical") }) {
		t.Fatal("critical post rejected")
	}
	if q.CriticalLen() != 1 {
		t.Fatalf("critical len = %d, want 1", q.CriticalLen())
	}

	if drained := q.Drain(2); drained != 2 {
		t.Fatalf("drained = %d, want 2", drained)
	}
	if len(got) != 2 || got[0] != "critical" || got[1] != "ordinary" {
		t.Fatalf("drain order = %v, want [critical ordinary]", got)
	}
}

func TestQueuePressureIsExplicit(t *testing.T) {
	q := New(1)
	if !q.TryPost(func() {}) {
		t.Fatal("first post rejected")
	}
	if q.TryPost(func() {}) {
		t.Fatal("second post should be rejected when queue is full")
	}
	if q.Len() != 1 || q.Cap() != 1 {
		t.Fatalf("queue size/capacity = %d/%d, want 1/1", q.Len(), q.Cap())
	}
}

func TestQueueRejectsNilTask(t *testing.T) {
	q := New(1)
	if q.TryPost(nil) {
		t.Fatal("nil ordinary task should be rejected")
	}
	if q.PostCritical(nil) {
		t.Fatal("nil critical task should be rejected")
	}
	if drained := q.Drain(1); drained != 0 {
		t.Fatalf("drained nil task count = %d, want 0", drained)
	}
}
