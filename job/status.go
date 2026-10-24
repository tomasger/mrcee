package job

import (
	"syscall"
	"time"
)

type Status struct {
	ID                string
	State             State
	Command           string
	Args              []string
	StartedAt         time.Time
	FinishedAt        *time.Time
	ExitCode          *int
	TerminationSignal *syscall.Signal
	// TODO: add other useful arguments e.g. CreatedBy
}
