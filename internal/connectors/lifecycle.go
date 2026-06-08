package connectors

import (
	"context"
	"log"
	"time"

	"github.com/ceoai/navi/connectors"
)

// StartAll starts all stored connector instances. Call after Create for each desired connector.
func (r *Registry) StartAll(ctx context.Context) {
	r.mu.RLock()
	instances := make(map[string]connectors.Connector)
	for k, v := range r.instances {
		instances[k] = v
	}
	r.mu.RUnlock()

	for name, conn := range instances {
		go func(n string, c connectors.Connector) {
			if err := c.Start(ctx); err != nil {
				log.Printf("connector %s stopped: %v", n, err)
			}
		}(name, conn)
		r.setStartedAt(name, time.Now().UTC())
		log.Printf("connector %s started", name)
	}
}

// StartOne starts the named connector if it exists. Used when a connector is added at runtime (e.g. wizard).
func (r *Registry) StartOne(ctx context.Context, name string) {
	conn := r.Get(name)
	if conn == nil {
		return
	}
	go func() {
		if err := conn.Start(ctx); err != nil {
			log.Printf("connector %s stopped: %v", name, err)
		}
	}()
	r.setStartedAt(name, time.Now().UTC())
	log.Printf("connector %s started", name)
}

// StopAll stops all connector instances with a timeout.
func (r *Registry) StopAll(ctx context.Context, timeout time.Duration) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	stopCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	r.mu.RLock()
	instances := make(map[string]connectors.Connector)
	for k, v := range r.instances {
		instances[k] = v
	}
	r.mu.RUnlock()

	for name, conn := range instances {
		if err := conn.Stop(stopCtx); err != nil {
			log.Printf("connector %s stop: %v", name, err)
		}
		r.mu.Lock()
		delete(r.startedAt, name)
		r.mu.Unlock()
	}
}
