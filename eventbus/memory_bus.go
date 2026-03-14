package eventbus

import (
	"context"
	"strings"
	"sync"
	"time"
)

// Event is a structured control-plane event.
type Event struct {
	ID        uint64 `json:"id"`
	Timestamp string `json:"timestamp"`
	Source    string `json:"source,omitempty"`
	Type      string `json:"type"`
	Kind      string `json:"kind,omitempty"`
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Action    string `json:"action,omitempty"`
	Message   string `json:"message,omitempty"`
	Data      any    `json:"data,omitempty"`
}

// Filter narrows event delivery for subscriptions.
type Filter struct {
	SinceID   uint64
	Source    string
	Type      string
	Kind      string
	Name      string
	Namespace string
}

// Bus provides publish/subscribe semantics for control-plane events.
type Bus interface {
	Publish(Event) Event
	Subscribe(context.Context, Filter) <-chan Event
	LatestID() uint64
}

// MemoryBus is an in-memory event bus with bounded history.
type MemoryBus struct {
	mu         sync.Mutex
	nextID     uint64
	nextSubID  uint64
	historyMax int
	history    []Event
	subs       map[uint64]*subscriber
}

type subscriber struct {
	ch     chan Event
	filter Filter
}

func NewMemoryBus(historyMax int) *MemoryBus {
	if historyMax <= 0 {
		historyMax = 2048
	}
	return &MemoryBus{
		historyMax: historyMax,
		history:    make([]Event, 0, historyMax),
		subs:       make(map[uint64]*subscriber),
	}
}

func (b *MemoryBus) Publish(evt Event) Event {
	b.mu.Lock()
	b.nextID++
	evt.ID = b.nextID
	if strings.TrimSpace(evt.Timestamp) == "" {
		evt.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	b.history = append(b.history, evt)
	if len(b.history) > b.historyMax {
		b.history = b.history[len(b.history)-b.historyMax:]
	}

	targets := make([]chan Event, 0, len(b.subs))
	for _, sub := range b.subs {
		if !matchesFilter(evt, sub.filter) {
			continue
		}
		targets = append(targets, sub.ch)
	}
	b.mu.Unlock()

	for _, ch := range targets {
		select {
		case ch <- evt:
		default:
			// Keep subscribers non-blocking; drop oldest buffered event on backpressure.
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- evt:
			default:
			}
		}
	}
	return evt
}

func (b *MemoryBus) Subscribe(ctx context.Context, filter Filter) <-chan Event {
	ch := make(chan Event, 256)

	b.mu.Lock()
	b.nextSubID++
	subID := b.nextSubID
	b.subs[subID] = &subscriber{ch: ch, filter: filter}

	snapshot := make([]Event, 0, len(b.history))
	for _, evt := range b.history {
		if evt.ID <= filter.SinceID {
			continue
		}
		if !matchesFilter(evt, filter) {
			continue
		}
		snapshot = append(snapshot, evt)
	}
	b.mu.Unlock()

	go func() {
		for _, evt := range snapshot {
			select {
			case <-ctx.Done():
				b.removeSubscriber(subID)
				close(ch)
				return
			case ch <- evt:
			}
		}
		<-ctx.Done()
		b.removeSubscriber(subID)
		close(ch)
	}()

	return ch
}

func (b *MemoryBus) LatestID() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.nextID
}

func (b *MemoryBus) removeSubscriber(id uint64) {
	b.mu.Lock()
	delete(b.subs, id)
	b.mu.Unlock()
}

func matchesFilter(evt Event, filter Filter) bool {
	if strings.TrimSpace(filter.Source) != "" && !strings.EqualFold(strings.TrimSpace(filter.Source), strings.TrimSpace(evt.Source)) {
		return false
	}
	if strings.TrimSpace(filter.Type) != "" && !strings.EqualFold(strings.TrimSpace(filter.Type), strings.TrimSpace(evt.Type)) {
		return false
	}
	if strings.TrimSpace(filter.Kind) != "" && !strings.EqualFold(strings.TrimSpace(filter.Kind), strings.TrimSpace(evt.Kind)) {
		return false
	}
	if strings.TrimSpace(filter.Name) != "" && !strings.EqualFold(strings.TrimSpace(filter.Name), strings.TrimSpace(evt.Name)) {
		return false
	}
	if strings.TrimSpace(filter.Namespace) != "" && !strings.EqualFold(strings.TrimSpace(filter.Namespace), strings.TrimSpace(evt.Namespace)) {
		return false
	}
	return true
}
