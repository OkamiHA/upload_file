package profile

import (
	"gitlab.viettelcyber.com/kian-core-v2/profiling-engine/node_processor"
	"time"
)

type AggWorker struct {
	*params
	commonWorker
	inChan        chan *node_processor.RawEvent
	batches       map[int64]*AggBatch
	maxBatch      int
	batchPkgChan  chan *AggBatch
	watermark     int
	tempWatermark int
	mapTimer      *MapTimer
}

type AggBatch struct {
	model            IAggModel
	numBuiltEvents   int
	roundedTimestamp int
}

type MapTimer struct {
	mapTimer              map[int64]*time.Timer
	duration              time.Duration
	cancelChan, forceChan map[int64]chan struct{}
	outChan               chan int64
}

func NewMapTimer(duration time.Duration) *MapTimer {
	return &MapTimer{
		mapTimer:   make(map[int64]*time.Timer),
		cancelChan: make(map[int64]chan struct{}),
		forceChan:  make(map[int64]chan struct{}),
		duration:   duration,
	}
}

func (t *MapTimer) CreateTimer(ts int64) {
	if _, ok := t.mapTimer[ts]; !ok {
		t.mapTimer[ts] = time.NewTimer(t.duration)
		t.cancelChan[ts] = make(chan struct{})
	}
}

func (t *MapTimer) DeleteTimer(ts int64) {
	t.cancelChan[ts] <- struct{}{}
}

func (t *MapTimer) ResetTimer(ts int64) {
	t.mapTimer[ts].Reset(t.duration)
}

func (t *MapTimer) ForceTimer(ts int64) {
	t.forceChan[ts] <- struct{}{}
}

func (t *MapTimer) Start(ts int64) {
	go func() {
		select {
		case <-t.mapTimer[ts].C:
			t.outChan <- ts
			t.mapTimer[ts].Reset(t.duration)
		case <-t.forceChan[ts]:
			t.outChan <- ts
			t.mapTimer[ts].Reset(t.duration)
		case <-t.cancelChan[ts]:
			t.mapTimer[ts].Stop()
			delete(t.mapTimer, ts)
			close(t.cancelChan[ts])
			delete(t.cancelChan, ts)
			break
		}
	}()
}

func (t *MapTimer) GetOutChan() chan int64 {
	return t.outChan
}

func NewAggWorker(maxBatch int, inChan chan *node_processor.RawEvent) *AggWorker {
	return &AggWorker{
		batches:      make(map[int64]*AggBatch, maxBatch),
		maxBatch:     maxBatch,
		inChan:       inChan,
		batchPkgChan: make(chan *AggBatch),
	}
}

func (w *AggWorker) Start() {
	watermarkTicker := time.NewTicker(time.Second * 30)
	earlyBuildTimer := time.NewTimer(w.params.hopDuration / 2)

	go func() {
		case msg := <- w.inChan:
			process(event)
			if event <
	}()
}

