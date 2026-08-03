package engine

import (
	"errors"
	"fmt"
)

var (
	errAlreadyRunning     = errors.New("engine: already running")
	errEmptyServerAddress = errors.New("engine: outbound server address is required")
	errEmptyServerPort    = errors.New("engine: outbound server port is required")
)

func errInvalidMode(mode Mode) error {
	return fmt.Errorf("engine: invalid mode %q", mode)
}
