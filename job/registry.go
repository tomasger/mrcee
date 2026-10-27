package job

import (
	"errors"
	"io"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrEmptyCommand      = errors.New("command is empty")
	ErrJobNotFound       = errors.New("job not found")
	ErrJobAlreadyStopped = errors.New("job already stopped")
)

type Registry struct {
	m    sync.RWMutex
	jobs map[string]*jobEntry
}

type jobEntry struct {
	runtime *job
	status  Status
	output  *output
}

func NewRegistry() *Registry {
	return &Registry{
		jobs: make(map[string]*jobEntry),
	}
}

// StartJob initializes a job that will execute the command
// with its given arguments. Returns job ID (UUIDv4).
func (r *Registry) StartJob(command string, args []string) (string, error) {
	if command == "" {
		return "", ErrEmptyCommand
	}

	// TODO: Support a configurable limit on running jobs and
	// add a rejection or queue mechanism

	id := uuid.NewString()
	o := &output{changed: make(chan struct{})}

	status := Status{
		ID:        id,
		State:     StateRunning,
		Command:   command,
		Args:      args,
		StartedAt: time.Now(),
	}

	job, err := startJob(command, args, o)
	if err != nil {
		return "", err
	}

	entry := &jobEntry{
		status:  status,
		runtime: job,
		output:  o,
	}

	r.m.Lock()
	r.jobs[id] = entry
	r.m.Unlock()

	go r.monitorJob(entry)
	return id, nil
}

func (r *Registry) monitorJob(entry *jobEntry) {
	finishedJobState := entry.runtime.wait()

	// update state
	r.m.Lock()
	entry.status.State = finishedJobState.state
	entry.status.FinishedAt = &finishedJobState.finishedAt
	entry.status.ExitCode = finishedJobState.exitCode
	entry.status.TerminationSignal = finishedJobState.terminationSignal
	r.m.Unlock()

	// Publish final status before readers receive EOF.
	entry.output.finish()

	// allow Registry.StopJob to finish
	entry.runtime.markDone()
}

func (r *Registry) JobStatus(id string) (Status, error) {
	r.m.RLock()
	defer r.m.RUnlock()
	entry, ok := r.jobs[id]
	if !ok {
		return Status{}, ErrJobNotFound
	}

	return entry.status, nil
}

// StopJob issues a SIGKILL to a job. (TODO: implement graceful exit)
// If a job is already finished, ErrJobAlreadyStopped is returned.
func (r *Registry) StopJob(id string) (Status, error) {
	r.m.RLock()
	entry, ok := r.jobs[id]
	if !ok {
		r.m.RUnlock()
		return Status{}, ErrJobNotFound
	}

	// check if job is finished before issuing the signal
	if entry.status.State != StateRunning {
		r.m.RUnlock()
		return Status{}, ErrJobAlreadyStopped
	}
	r.m.RUnlock()

	err := entry.runtime.stop()
	// job may have finished in the time it took to send the signal
	if errors.Is(err, os.ErrProcessDone) {
		return Status{}, ErrJobAlreadyStopped
	}
	if err != nil {
		return Status{}, err
	}

	// wait for Registry.monitorJob to finish
	<-entry.runtime.done

	// re-fetch status
	return r.JobStatus(id)
}

// OpenStream replays a job's output from the beginning, then waits for new
// output until the job finishes. Closing the stream does not stop the job.
func (r *Registry) OpenStream(id string) (io.ReadCloser, error) {
	r.m.RLock()
	defer r.m.RUnlock()

	entry, ok := r.jobs[id]
	if !ok {
		return nil, ErrJobNotFound
	}

	return &outputReader{
		output:       entry.output,
		readerClosed: make(chan struct{}),
	}, nil
}

// TODO: Add a ListJobs to list all the jobs
// TODO: Add shutdown support that rejects new jobs and applies
// and stops or waits for running jobs.
