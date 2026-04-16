package agentruntime

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type memoryDelivery struct {
	message AgentMessage
	ackFn   func(bool, time.Duration) error
}

func (d *memoryDelivery) Message() AgentMessage {
	return d.message
}

func (d *memoryDelivery) Ack(_ context.Context) error {
	if d.ackFn == nil {
		return nil
	}
	return d.ackFn(false, 0)
}

func (d *memoryDelivery) Nack(_ context.Context, requeue bool) error {
	if d.ackFn == nil {
		return nil
	}
	return d.ackFn(requeue, 0)
}

func (d *memoryDelivery) NackWithDelay(_ context.Context, delay time.Duration) error {
	if d.ackFn == nil {
		return nil
	}
	return d.ackFn(true, delay)
}

func (d *memoryDelivery) ExtendLease(_ context.Context, _ time.Duration) error {
	return nil
}

type memorySubscriber struct {
	id      uint64
	subject string
	ch      chan AgentMessage
}

// MemoryAgentMessageBus is an in-process runtime message bus for local dev/test.
type MemoryAgentMessageBus struct {
	mu            sync.Mutex
	nextSubID     uint64
	subjectPrefix string
	historyMax    int
	dedupeWindow  time.Duration
	history       []AgentMessage
	dedupe        map[string]time.Time
	subs          map[uint64]*memorySubscriber
	done          chan struct{}
	closeOnce     sync.Once
}

func NewMemoryAgentMessageBus(subjectPrefix string, historyMax int, dedupeWindow time.Duration) *MemoryAgentMessageBus {
	if historyMax <= 0 {
		historyMax = 2048
	}
	if dedupeWindow <= 0 {
		dedupeWindow = 2 * time.Minute
	}
	return &MemoryAgentMessageBus{
		subjectPrefix: subjectPrefix,
		historyMax:    historyMax,
		dedupeWindow:  dedupeWindow,
		history:       make([]AgentMessage, 0, historyMax),
		dedupe:        make(map[string]time.Time),
		subs:          make(map[uint64]*memorySubscriber),
		done:          make(chan struct{}),
	}
}

func (b *MemoryAgentMessageBus) Publish(_ context.Context, message AgentMessage) (AgentMessage, error) {
	normalized, err := normalizeAgentMessage(message)
	if err != nil {
		return AgentMessage{}, err
	}
	if b.isClosed() {
		return normalized, nil
	}
	subject := messageSubject(b.subjectPrefix, normalized.Namespace, normalized.ToAgent)

	b.mu.Lock()
	if b.isClosed() {
		b.mu.Unlock()
		return normalized, nil
	}
	b.pruneDedupeLocked()
	if seenAt, exists := b.dedupe[normalized.MessageID]; exists {
		if time.Since(seenAt) <= b.dedupeWindow {
			b.mu.Unlock()
			return normalized, nil
		}
	}
	b.dedupe[normalized.MessageID] = time.Now().UTC()
	b.history = append(b.history, normalized)
	if len(b.history) > b.historyMax {
		b.history = b.history[len(b.history)-b.historyMax:]
	}
	targets := make([]chan AgentMessage, 0, len(b.subs))
	for _, sub := range b.subs {
		if sub.subject != subject {
			continue
		}
		targets = append(targets, sub.ch)
	}
	b.mu.Unlock()

	for _, ch := range targets {
		b.enqueueNonBlocking(ch, normalized)
	}
	return normalized, nil
}

func (b *MemoryAgentMessageBus) Consume(ctx context.Context, sub AgentMessageSubscription, handler AgentMessageHandler) error {
	if handler == nil {
		return fmt.Errorf("handler is required")
	}
	namespace := sub.Namespace
	if namespace == "" {
		namespace = "default"
	}
	subject := messageSubject(b.subjectPrefix, namespace, sub.Agent)
	consumer := &memorySubscriber{
		subject: subject,
		ch:      make(chan AgentMessage, 256),
	}

	b.mu.Lock()
	if b.isClosed() {
		b.mu.Unlock()
		return nil
	}
	b.nextSubID++
	consumer.id = b.nextSubID
	b.subs[consumer.id] = consumer
	replay := make([]AgentMessage, 0, len(b.history))
	for _, historical := range b.history {
		if messageSubject(b.subjectPrefix, historical.Namespace, historical.ToAgent) != subject {
			continue
		}
		replay = append(replay, historical)
	}
	b.mu.Unlock()
	defer b.removeSubscriber(consumer.id)
	for _, historical := range replay {
		b.enqueueNonBlocking(consumer.ch, historical)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-b.done:
			return nil
		case message := <-consumer.ch:
			msg := message
			delivery := &memoryDelivery{
				message: msg,
				ackFn: func(requeue bool, delay time.Duration) error {
					if !requeue {
						return nil
					}
					b.requeueToSubscriber(consumer.id, msg, delay)
					return nil
				},
			}
			if err := handler(ctx, delivery); err != nil {
				if delay, ok := retryDelayFromError(err); ok {
					_ = delivery.NackWithDelay(ctx, delay)
				} else {
					_ = delivery.Nack(ctx, true)
				}
				continue
			}
			_ = delivery.Ack(ctx)
		}
	}
}

func (b *MemoryAgentMessageBus) Close() error {
	b.closeOnce.Do(func() {
		close(b.done)
		b.mu.Lock()
		defer b.mu.Unlock()
		for id := range b.subs {
			delete(b.subs, id)
		}
	})
	return nil
}

func (b *MemoryAgentMessageBus) removeSubscriber(id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subs, id)
}

func (b *MemoryAgentMessageBus) pruneDedupeLocked() {
	cutoff := time.Now().UTC().Add(-b.dedupeWindow)
	for messageID, seenAt := range b.dedupe {
		if seenAt.Before(cutoff) {
			delete(b.dedupe, messageID)
		}
	}
}

func (b *MemoryAgentMessageBus) requeueToSubscriber(id uint64, message AgentMessage, delay time.Duration) {
	if b.isClosed() {
		return
	}
	if delay <= 0 {
		b.pushToSubscriber(id, message)
		return
	}
	timer := time.NewTimer(delay)
	go func() {
		defer timer.Stop()
		select {
		case <-b.done:
			return
		case <-timer.C:
			b.pushToSubscriber(id, message)
		}
	}()
}

func (b *MemoryAgentMessageBus) pushToSubscriber(id uint64, message AgentMessage) {
	if b.isClosed() {
		return
	}
	b.mu.Lock()
	sub, ok := b.subs[id]
	b.mu.Unlock()
	if !ok || sub == nil {
		return
	}
	select {
	case <-b.done:
		return
	case sub.ch <- message:
	default:
	}
}

func (b *MemoryAgentMessageBus) enqueueNonBlocking(ch chan AgentMessage, message AgentMessage) {
	if ch == nil || b.isClosed() {
		return
	}
	select {
	case <-b.done:
		return
	case ch <- message:
	default:
		// Keep publish/replay non-blocking.
		select {
		case <-b.done:
			return
		case <-ch:
		default:
		}
		select {
		case <-b.done:
			return
		case ch <- message:
		default:
		}
	}
}

func (b *MemoryAgentMessageBus) isClosed() bool {
	select {
	case <-b.done:
		return true
	default:
		return false
	}
}
