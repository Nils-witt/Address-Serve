package collector

import (
	"testing"

	"github.com/google/uuid"

	"github.com/nils-witt/address-serve/internal/store"
)

func TestBroadcasterDeliversWithoutIssuesAndLog(t *testing.T) {
	t.Parallel()

	b := newBroadcaster()
	a, unsubA := b.subscribe()
	c, unsubC := b.subscribe()

	defer unsubA()
	defer unsubC()

	run := store.CollectorRun{
		ID: uuid.New(), Status: store.RunRunning, Log: "output", Issues: []store.SyncIssue{{}},
	}
	b.publish(run)

	for name, ch := range map[string]<-chan store.CollectorRun{"a": a, "c": c} {
		got := <-ch
		if got.ID != run.ID || got.Log != "" || got.Issues != nil {
			t.Errorf("subscriber %s got %+v, want run %s without issues and log", name, got, run.ID)
		}
	}
}

func TestBroadcasterUnsubscribe(t *testing.T) {
	t.Parallel()

	b := newBroadcaster()
	ch, unsubscribe := b.subscribe()

	unsubscribe()
	unsubscribe() // a second call must not close the channel again

	if _, ok := <-ch; ok {
		t.Fatal("channel still open after unsubscribe")
	}

	b.publish(store.CollectorRun{ID: uuid.New()}) // must not panic on a closed channel
}

func TestBroadcasterDropsSlowSubscriber(t *testing.T) {
	t.Parallel()

	b := newBroadcaster()
	slow, unsubSlow := b.subscribe()

	defer unsubSlow()

	for range eventBuffer + 1 {
		b.publish(store.CollectorRun{ID: uuid.New()})
	}

	n := 0
	for range slow {
		n++
	}

	if n != eventBuffer {
		t.Errorf("slow subscriber got %d updates before being dropped, want %d", n, eventBuffer)
	}

	// The dropped subscriber is gone, so another one still gets updates.
	fresh, unsubFresh := b.subscribe()
	defer unsubFresh()

	b.publish(store.CollectorRun{ID: uuid.New()})

	if _, ok := <-fresh; !ok {
		t.Error("fresh subscriber got no update")
	}
}
