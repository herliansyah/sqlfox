package driver

import (
	"context"
	"fmt"
)

var drivers = make(map[string]Driver)

// Register registers a database driver by name.
// ponytail: Simple map without mutex; driver registration occurs strictly during init().
func Register(name string, d Driver) {
	drivers[name] = d
}

// GetDriver returns the driver registered with the given name.
func GetDriver(name string) (Driver, error) {
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
