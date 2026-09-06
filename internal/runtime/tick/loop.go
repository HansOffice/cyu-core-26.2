package tick

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

const DefaultRate = 20

type Callback func()

type Snapshot struct {
	TargetTPS   float64
	TPS         float64
	LastMSPT    float64
	AverageMSPT float64
	TotalTicks  uint64
}

type Loop struct {
	rate     int
	interval time.Duration
	callback Callback

	lifecycleMu sync.Mutex
	running     atomic.Bool
	stopCh      chan struct{}
	doneCh      chan struct{}

	totalTicks      atomic.Uint64
	lastDurationNS  atomic.Int64
	averageMSPTBits atomic.Uint64
	observedTPSBits atomic.Uint64
}

func New(rate int, callback Callback) *Loop {
	if rate <= 0 {
		panic("tick: rate must be positive")
	}
	if callback == nil {
		panic("tick: callback must not be nil")
	}
	return &Loop{
		rate:     rate,
		interval: time.Second / time.Duration(rate),
		callback: callback,
	}
}

func (l *Loop) Start() bool {
	l.lifecycleMu.Lock()
	defer l.lifecycleMu.Unlock()

	if l.running.Load() {
		return false
	}

	l.stopCh = make(chan struct{})
	l.doneCh = make(chan struct{})
	l.running.Store(true)
	go l.run(l.stopCh, l.doneCh)
	return true
}

func (l *Loop) Stop() bool {
	l.lifecycleMu.Lock()
	if !l.running.Load() {
		l.lifecycleMu.Unlock()
		return false
	}
	stopCh := l.stopCh
	doneCh := l.doneCh
	l.running.Store(false)
	close(stopCh)
	l.lifecycleMu.Unlock()

	<-doneCh
	return true
}

func (l *Loop) Running() bool {
	return l.running.Load()
}

func (l *Loop) Snapshot() Snapshot {
	return Snapshot{
		TargetTPS:   float64(l.rate),
		TPS:         math.Float64frombits(l.observedTPSBits.Load()),
		LastMSPT:    float64(l.lastDurationNS.Load()) / float64(time.Millisecond),
		AverageMSPT: math.Float64frombits(l.averageMSPTBits.Load()),
		TotalTicks:  l.totalTicks.Load(),
	}
}

func (l *Loop) run(stopCh <-chan struct{}, doneCh chan<- struct{}) {
	defer close(doneCh)

	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()

	windowStarted := time.Now()
	windowTicks := uint64(0)

	for {
		select {
		case <-ticker.C:
			started := time.Now()
			l.callback()
			elapsed := time.Since(started)

			l.recordDuration(elapsed)
			l.totalTicks.Add(1)
			windowTicks++

			now := time.Now()
			windowElapsed := now.Sub(windowStarted)
			if windowElapsed >= time.Second {
				observed := float64(windowTicks) / windowElapsed.Seconds()
				l.observedTPSBits.Store(math.Float64bits(observed))
				windowStarted = now
				windowTicks = 0
			}

		case <-stopCh:
			return
		}
	}
}

func (l *Loop) recordDuration(duration time.Duration) {
	l.lastDurationNS.Store(duration.Nanoseconds())
	sample := float64(duration) / float64(time.Millisecond)

	for {
		oldBits := l.averageMSPTBits.Load()
		old := math.Float64frombits(oldBits)
		next := sample
		if old > 0 {
			const smoothing = 0.1
			next = old*(1-smoothing) + sample*smoothing
		}
		if l.averageMSPTBits.CompareAndSwap(oldBits, math.Float64bits(next)) {
			return
		}
	}
}
