package controllers

import (
	"context"
	"strings"
	"sync"
)

type keyQueue struct {
	ch      chan string
	mu      sync.Mutex
	pending map[string]struct{}
}

func newKeyQueue(size int) *keyQueue {
	if size <= 0 {
		size = 1024
	}
	return &keyQueue{
		ch:      make(chan string, size),
		pending: make(map[string]struct{}),
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
	q.mu.Unlock()
	q.ch <- key
}

func (q *keyQueue) Pop(ctx context.Context) (string, bool) {
	select {
	case <-ctx.Done():
		return "", false
	case key := <-q.ch:
		return key, true
	}
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
