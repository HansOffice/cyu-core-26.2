package tick

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestNewRejectsInvalidArguments(t *testing.T) {
	t.Run("rate", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic for non-positive rate")
			}
		}()
		New(0, func() {})
	})

	t.Run("callback", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic for nil callback")
			}
		}()
		New(DefaultRate, nil)
	})
}

func TestLoopStartsTicksAndStops(t *testing.T) {
	var ticks atomic.Int64
	loop := New(200, func() {
		ticks.Add(1)
	})

	if !loop.Start() {
		t.Fatal("expected first Start to start the loop")
	}
	if loop.Start() {
		t.Fatal("expected second Start to be ignored")
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for ticks.Load() < 5 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if ticks.Load() < 5 {
		t.Fatalf("expected at least 5 ticks, got %d", ticks.Load())
	}

	if !loop.Stop() {
		t.Fatal("expected first Stop to stop the loop")
	}
	if loop.Stop() {
		t.Fatal("expected second Stop to be ignored")
	}

	stoppedAt := ticks.Load()
	time.Sleep(20 * time.Millisecond)
	if got := ticks.Load(); got != stoppedAt {
		t.Fatalf("loop ticked after Stop: before=%d after=%d", stoppedAt, got)
	}
}

func TestRecordDurationUpdatesMetrics(t *testing.T) {
	loop := New(DefaultRate, func() {})

	loop.recordDuration(2 * time.Millisecond)
	first := loop.Snapshot()
	if first.LastMSPT != 2 {
		t.Fatalf("expected last MSPT 2, got %f", first.LastMSPT)
	}
	if first.AverageMSPT != 2 {
		t.Fatalf("expected average MSPT 2, got %f", first.AverageMSPT)
	}

	loop.recordDuration(4 * time.Millisecond)
	second := loop.Snapshot()
	if second.LastMSPT != 4 {
		t.Fatalf("expected last MSPT 4, got %f", second.LastMSPT)
	}
	if second.AverageMSPT <= 2 || second.AverageMSPT >= 4 {
		t.Fatalf("expected smoothed average between 2 and 4, got %f", second.AverageMSPT)
	}
}
