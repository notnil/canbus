package canbus

import (
	"sync"
)

// LoopbackBus is an in-memory CAN bus for tests and simulations.
// Multiple endpoints opened from the same bus can exchange frames.
type LoopbackBus struct {
	mu        sync.RWMutex
	closed    bool
	endpoints map[*loopEndpoint]struct{}
}

// NewLoopbackBus creates a new loopback bus.
func NewLoopbackBus() *LoopbackBus {
	return &LoopbackBus{endpoints: make(map[*loopEndpoint]struct{})}
}

// Open creates a new endpoint attached to the bus.
func (b *LoopbackBus) Open() Bus {
	ep := &loopEndpoint{
		bus:    b,
		ch:     make(chan Frame, 64),
		closed: make(chan struct{}),
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		close(ep.closed)
		return ep
	}
	b.endpoints[ep] = struct{}{}
	b.mu.Unlock()
	return ep
}

// Close closes the bus and detaches all endpoints.
func (b *LoopbackBus) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	for ep := range b.endpoints {
		ep.closeNoLock()
	}
	b.endpoints = nil
	b.mu.Unlock()
	return nil
}

type loopEndpoint struct {
	bus    *LoopbackBus
	ch     chan Frame
	mu     sync.Mutex
	dead   bool
	closed chan struct{}
}

// Send broadcasts the frame to all other endpoints on the same bus.
func (e *loopEndpoint) Send(frame Frame) error {
	if err := frame.Validate(); err != nil {
		return err
	}
	e.mu.Lock()
	if e.dead {
		e.mu.Unlock()
		return ErrClosed
	}
	e.mu.Unlock()
	// Snapshot endpoints under bus lock to avoid holding while sending.
	e.bus.mu.RLock()
	if e.bus.closed {
		e.bus.mu.RUnlock()
		return ErrClosed
	}
	targets := make([]*loopEndpoint, 0, len(e.bus.endpoints))
	for ep := range e.bus.endpoints {
		if ep != e {
			targets = append(targets, ep)
		}
	}
	e.bus.mu.RUnlock()

	// Deliver to targets.
	for _, t := range targets {
		select {
		case t.ch <- frame:
		case <-t.closed:
		}
	}
	return nil
}

// Receive waits for the next frame.
func (e *loopEndpoint) Receive() (Frame, error) {
	f, ok := <-e.ch
	if !ok {
		return Frame{}, ErrClosed
	}
	return f, nil
}

// Close detaches endpoint from bus and closes its channel.
func (e *loopEndpoint) Close() error {
	e.bus.mu.Lock()
	e.closeNoLock()
	e.bus.mu.Unlock()
	return nil
}

func (e *loopEndpoint) closeNoLock() {
	e.mu.Lock()
	if e.dead {
		e.mu.Unlock()
		return
	}
	e.dead = true
	close(e.closed)
	close(e.ch)
	if e.bus.endpoints != nil {
		delete(e.bus.endpoints, e)
	}
	e.mu.Unlock()
}

