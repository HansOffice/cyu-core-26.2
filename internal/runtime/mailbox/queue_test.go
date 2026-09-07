package mailbox

import "testing"

func TestQueuePopsFIFO(t *testing.T) {
	q := New[int](4)
	for i := 1; i <= 3; i++ {
		if !q.TryPost(i) {
			t.Fatalf("post %d rejected", i)
		}
	}

	for want := 1; want <= 3; want++ {
		got, ok := q.TryPop()
		if !ok {
			t.Fatalf("pop %d reported empty", want)
		}
		if got != want {
			t.Fatalf("pop = %d, want %d", got, want)
		}
	}
	if _, ok := q.TryPop(); ok {
		t.Fatal("empty queue returned a value")
	}
}

func TestCriticalValuesSurviveOrdinaryPressureAndPopFirst(t *testing.T) {
	q := New[string](1)
	if !q.TryPost("ordinary") {
		t.Fatal("ordinary post rejected")
	}
	if q.TryPost("overflow") {
		t.Fatal("ordinary queue should be full")
	}
	if !q.PostCritical("critical") {
		t.Fatal("critical post rejected")
	}
	if q.CriticalLen() != 1 {
		t.Fatalf("critical len = %d, want 1", q.CriticalLen())
	}

	got, ok := q.TryPop()
	if !ok || got != "critical" {
		t.Fatalf("first pop = %q, %v; want critical, true", got, ok)
	}
	got, ok = q.TryPop()
	if !ok || got != "ordinary" {
		t.Fatalf("second pop = %q, %v; want ordinary, true", got, ok)
	}
}

func TestQueuePressureIsExplicit(t *testing.T) {
	q := New[int](1)
	if !q.TryPost(1) {
		t.Fatal("first post rejected")
	}
	if q.TryPost(2) {
		t.Fatal("second post should be rejected when queue is full")
	}
	if q.Len() != 1 || q.Cap() != 1 {
		t.Fatalf("queue size/capacity = %d/%d, want 1/1", q.Len(), q.Cap())
	}
}

func TestNilQueueIsSafe(t *testing.T) {
	var q *Queue[int]
	if q.TryPost(1) {
		t.Fatal("nil queue accepted ordinary value")
	}
	if q.PostCritical(1) {
		t.Fatal("nil queue accepted critical value")
	}
	if _, ok := q.TryPop(); ok {
		t.Fatal("nil queue returned a value")
	}
	if q.Len() != 0 || q.CriticalLen() != 0 || q.Cap() != 0 {
		t.Fatal("nil queue reported non-zero metadata")
	}
}
