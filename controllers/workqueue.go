package controllers

import (
	"context"
	"strings"
	"sync"
)

type keyQueue struct {
	mu      sync.Mutex
	pending map[string]struct{}
	items   []string
	notify  chan struct{}
}

func newKeyQueue(size int) *keyQueue {
	if size <= 0 {
		size = 1024
	}
	return &keyQueue{
		pending: make(map[string]struct{}),
		items:   make([]string, 0, size),
		notify:  make(chan struct{}, 1),
	}
}

func (q *keyQueue) Enqueue(key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	q.mu.Lock()
	if _, ok := q.pending[key]; ok {
		q.mu.Unlock()
		return
	}
	q.pending[key] = struct{}{}
	q.items = append(q.items, key)
	q.mu.Unlock()
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

func (q *keyQueue) Pop(ctx context.Context) (string, bool) {
	for {
		if key, ok := q.TryPop(); ok {
			return key, true
		}

		select {
		case <-ctx.Done():
			return "", false
		case <-q.notify:
		}
	}
}

func (q *keyQueue) TryPop() (string, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return "", false
	}
	key := q.items[0]
	q.items = q.items[1:]
	return key, true
}

func (q *keyQueue) Done(key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	q.mu.Lock()
	delete(q.pending, key)
	q.mu.Unlock()
}
