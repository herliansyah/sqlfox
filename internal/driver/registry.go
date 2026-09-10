package driver

import (
	"context"
	"fmt"
	"sync"
)

var (
	registryMu sync.RWMutex
	drivers    = make(map[string]Driver)
)

// Register registers a database driver by name.
func Register(name string, d Driver) {
	registryMu.Lock()
	defer registryMu.Unlock()
	drivers[name] = d
}

// GetDriver returns the driver registered with the given name.
func GetDriver(name string) (Driver, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	d, ok := drivers[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedDriver, name)
	}
	return d, nil
}

// Connect opens a session to the target database using the driver specified in cfg.
func Connect(ctx context.Context, cfg Config) (Session, error) {
	d, err := GetDriver(cfg.Driver)
	if err != nil {
		return nil, err
	}
	return d.Connect(ctx, cfg)
}
