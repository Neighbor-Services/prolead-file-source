package service

import (
	"sync"
	"time"
)

type EventType string

const (
	EventFileUploaded  EventType = "file:uploaded"
	EventFileDeleted   EventType = "file:deleted"
	EventTokenRotated  EventType = "token:rotated"
	EventBucketCreated EventType = "bucket:created"
)

type Event struct {
	Type      EventType   `json:"type"`
	Bucket    string      `json:"bucket"`
	Path      string      `json:"path,omitempty"`
	Payload   interface{} `json:"payload,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

type EventHub struct {
	mu          sync.RWMutex
	subscribers map[chan Event]struct{}
}

func NewEventHub() *EventHub {
	return &EventHub{
		subscribers: make(map[chan Event]struct{}),
	}
}

func (h *EventHub) Subscribe() chan Event {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan Event, 64)
	h.subscribers[ch] = struct{}{}
	return ch
}

func (h *EventHub) Unsubscribe(ch chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.subscribers[ch]; ok {
		delete(h.subscribers, ch)
		close(ch)
	}
}

func (h *EventHub) Publish(eventType EventType, bucket, path string, payload interface{}) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	event := Event{
		Type:      eventType,
		Bucket:    bucket,
		Path:      path,
		Payload:   payload,
		Timestamp: time.Now().UTC(),
	}

	for ch := range h.subscribers {
		select {
		case ch <- event:
		default:
			// Subscriber buffer full, non-blocking skip
		}
	}
}
