package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

const defaultNATSSubjectPrefix = "orloj.controlplane"

type natsEnvelope struct {
	Origin string `json:"origin"`
	Event  Event  `json:"event"`
}

// NATSBus distributes events across processes via NATS while preserving
// local replay/filter semantics through an in-memory cache.
type NATSBus struct {
	local   *MemoryBus
	nc      *nats.Conn
	subject string
	origin  string
	logger  *log.Logger

	mu     sync.Mutex
	closed bool
}

func NewNATSBus(url, subjectPrefix string, historyMax int, logger *log.Logger) (*NATSBus, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		url = nats.DefaultURL
	}
	subjectPrefix = strings.TrimSpace(subjectPrefix)
	if subjectPrefix == "" {
		subjectPrefix = defaultNATSSubjectPrefix
	}

	origin := defaultNATSOrigin()
	bus := &NATSBus{
		local:   NewMemoryBus(historyMax),
		subject: subjectPrefix + ".events",
		origin:  origin,
		logger:  logger,
	}

	nc, err := nats.Connect(url,
		nats.Name("orloj-eventbus"),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("connect nats %q: %w", url, err)
	}
	bus.nc = nc

	if _, err := bus.nc.Subscribe(bus.subject, bus.onMessage); err != nil {
		bus.nc.Close()
		return nil, fmt.Errorf("subscribe nats subject %q: %w", bus.subject, err)
	}

	if err := bus.nc.Flush(); err != nil {
		bus.nc.Close()
		return nil, fmt.Errorf("flush nats subscription: %w", err)
	}

	if bus.logger != nil {
		bus.logger.Printf("event bus backend=nats url=%s subject=%s origin=%s", url, bus.subject, bus.origin)
	}
	return bus, nil
}

func (b *NATSBus) Publish(evt Event) Event {
	if b == nil {
		return evt
	}
	published := b.local.Publish(evt)
	if b.nc == nil {
		return published
	}

	payload, err := json.Marshal(natsEnvelope{Origin: b.origin, Event: published})
	if err != nil {
		if b.logger != nil {
			b.logger.Printf("event bus nats marshal failed: %v", err)
		}
		return published
	}
	if err := b.nc.Publish(b.subject, payload); err != nil && b.logger != nil {
		b.logger.Printf("event bus nats publish failed: %v", err)
	}
	return published
}

func (b *NATSBus) Subscribe(ctx context.Context, filter Filter) <-chan Event {
	return b.local.Subscribe(ctx, filter)
}

func (b *NATSBus) LatestID() uint64 {
	return b.local.LatestID()
}

func (b *NATSBus) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	nc := b.nc
	b.mu.Unlock()

	if nc != nil {
		nc.Close()
	}
	return nil
}

func (b *NATSBus) onMessage(msg *nats.Msg) {
	if b == nil {
		return
	}
	var envelope natsEnvelope
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		if b.logger != nil {
			b.logger.Printf("event bus nats unmarshal failed: %v", err)
		}
		return
	}
	if strings.EqualFold(strings.TrimSpace(envelope.Origin), strings.TrimSpace(b.origin)) {
		return
	}
	b.local.Publish(envelope.Event)
}

func defaultNATSOrigin() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		host = "orloj"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}
