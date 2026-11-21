package collector

import (
	"sync"

	"github.com/nils-witt/address-serve/internal/store"
)

// eventBuffer is how many run updates a subscriber may fall behind by before
// it is dropped. Updates come at most every log flush interval per run, so
// only a stalled connection fills it.
const eventBuffer = 32

// broadcaster fans run updates out to subscribers. A subscriber that falls
// behind is dropped, its channel closed, rather than slowing the runs down:
// it is expected to reconnect and reload what it shows.
type broadcaster struct {
	mu   sync.Mutex
	subs map[chan store.CollectorRun]struct{}
}

func newBroadcaster() *broadcaster {
	return &broadcaster{subs: map[chan store.CollectorRun]struct{}{}}
}

// subscribe returns a channel of run updates and a func that ends the
// subscription. The channel is closed once it ends, or once it fell behind.
func (b *broadcaster) subscribe() (updates <-chan store.CollectorRun, unsubscribe func()) {
	ch := make(chan store.CollectorRun, eventBuffer)

	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()

		b.drop(ch)
	}
}

// publish sends run, without its issues and log, to every subscriber.
func (b *broadcaster) publish(run store.CollectorRun) {
	run.Issues, run.Log = nil, ""

	b.mu.Lock()
	defer b.mu.Unlock()

	for ch := range b.subs {
		select {
		case ch <- run:
		default:
			b.drop(ch)
		}
	}
}

// drop ends a subscription once; b.mu must be held.
func (b *broadcaster) drop(ch chan store.CollectorRun) {
	if _, ok := b.subs[ch]; ok {
		delete(b.subs, ch)
		close(ch)
	}
}
