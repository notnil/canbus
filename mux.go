package canbus

import (
	"sync"
)

// FrameFilter decides whether a frame should be delivered to a subscriber.
type FrameFilter func(Frame) bool

// Mux multiplexes frames from a Bus to any number of subscribers via filters.
//
// It owns the provided Bus instance for receiving and runs a single background
// goroutine to read from Receive and fan-out frames to subscribers. This avoids
// having multiple goroutines competing to Receive and enables non-blocking,
// filtered consumption for higher-level protocols like CANopen SDO.
//
// Send is not proxied; callers should keep using the original Bus to Send.
type Mux struct {
	bus   Bus
	stop  chan struct{}

	mu    sync.RWMutex
	subs  map[uint64]*subscriber
	next  uint64
}

type subscriber struct {
	filter FrameFilter
	ch     chan Frame
}

// NewMux creates and starts a multiplexer bound to the given Bus.
func NewMux(bus Bus) *Mux {
	m := &Mux{
		bus:  bus,
		stop: make(chan struct{}),
		subs: make(map[uint64]*subscriber),
	}
	go m.run()
	return m
}

// Close stops the background reader and closes all subscriber channels.
func (m *Mux) Close() error {
	select {
	case <-m.stop:
		return nil
	default:
	}
	close(m.stop)
	// Best-effort drain/close of subscribers
	m.mu.Lock()
	for id, s := range m.subs {
		close(s.ch)
		delete(m.subs, id)
	}
	m.mu.Unlock()
	return nil
}

// Subscribe registers a new subscriber with the provided filter and channel buffer.
// The returned channel will receive frames that match the filter. The cancel
// function should be called when no longer needed; it will close the channel.
func (m *Mux) Subscribe(filter FrameFilter, buffer int) (<-chan Frame, func()) {
	if buffer < 0 {
		buffer = 0
	}
	s := &subscriber{filter: filter, ch: make(chan Frame, buffer)}
	m.mu.Lock()
	id := m.next
	m.next++
	m.subs[id] = s
	m.mu.Unlock()

	cancel := func() {
		m.mu.Lock()
		if cur, ok := m.subs[id]; ok && cur == s {
			close(cur.ch)
			delete(m.subs, id)
		}
		m.mu.Unlock()
	}
	return s.ch, cancel
}

func (m *Mux) run() {
	for {
		select {
		case <-m.stop:
			return
		default:
		}
		f, err := m.bus.Receive()
		if err != nil {
			// On error, propagate closure to subscribers and exit.
			m.mu.Lock()
			for id, s := range m.subs {
				close(s.ch)
				delete(m.subs, id)
			}
			m.mu.Unlock()
			return
		}
		m.mu.RLock()
		for _, s := range m.subs {
			if s.filter == nil || s.filter(f) {
				select {
				case s.ch <- f:
				default:
					// Drop if subscriber is slow and channel is full.
				}
			}
		}
		m.mu.RUnlock()
	}
}

