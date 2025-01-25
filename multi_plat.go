package blockuntilclosed

import (
	"log"
)

// MultiplexPlatform is the interface for the platform-specific implementation of the MultiplexBackend.
type MultiplexPlatform interface {
	Start(cancelFD int) (int, error)
	SetLogger(logger *log.Logger)
	Subscribe(pollfd, fd int) error
	Recv(pollFD int) (fd int, isClose bool, _ error)
}
