package tick

import (
	"sync"
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

func TestLoopCanRestartAfterStop(t *testing.T) {
	var ticks atomic.Int64
	loop := New(200, func() {
		ticks.Add(1)
	})

	if !loop.Start() {
		t.Fatal("expected initial Start to succeed")
	}
	waitForTicks(t, &ticks, 2)
	if !loop.Stop() {
		t.Fatal("expected Stop to succeed")
	}

	beforeRestart := ticks.Load()
	if !loop.Start() {
		t.Fatal("expected Start after completed Stop to succeed")
	}
	waitForTicks(t, &ticks, beforeRestart+2)
	if !loop.Stop() {
		t.Fatal("expected second Stop to succeed")
	}
}

func TestLoopRejectsStartWhileStopping(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once

	loop := New(1000, func() {
		once.Do(func() {
			close(entered)
			<-release
		})
	})

	if !loop.Start() {
		t.Fatal("expected Start to succeed")
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("tick callback did not start")
	}

	stopResult := make(chan bool, 1)
	go func() {
		stopResult <- loop.Stop()
	}()

	deadline := time.Now().Add(time.Second)
	for {
		loop.lifecycleMu.Lock()
		stopping := loop.stopping
		loop.lifecycleMu.Unlock()
		if stopping {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("loop did not enter stopping state")
		}
		time.Sleep(time.Millisecond)
	}

	if loop.Start() {
		t.Fatal("Start must not create a second loop while Stop is waiting for the current loop")
	}

	close(release)
	select {
	case stopped := <-stopResult:
		if !stopped {
			t.Fatal("expected Stop to own the shutdown")
		}
	case <-time.After(time.Second):
		t.Fatal("Stop did not complete")
	}

	if !loop.Start() {
		t.Fatal("expected restart after completed Stop")
	}
	if !loop.Stop() {
		t.Fatal("expected final Stop to succeed")
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

func waitForTicks(t *testing.T, ticks *atomic.Int64, want int64) {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for ticks.Load() < want && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := ticks.Load(); got < want {
		t.Fatalf("expected at least %d ticks, got %d", want, got)
	}
}
