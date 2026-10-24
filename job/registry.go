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
	m        sync.RWMutex
	jobStore map[string]*job
	// TODO: wrap statusStore in a struct, expose as interface
	statusStore map[string]Status
	// TODO: wrap outputStore in a struct, expose as interface
	outputStore map[string]*output
}

func NewRegistry() *Registry {
	return &Registry{
		jobStore:    make(map[string]*job),
		statusStore: make(map[string]Status),
		outputStore: make(map[string]*output),
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

	r.m.Lock()

	r.statusStore[id] = status
	r.outputStore[id] = o
	r.jobStore[id] = job
	r.m.Unlock()

	go r.monitorJob(id, job, o)
	return id, nil
}

func (r *Registry) monitorJob(id string, j *job, o *output) {
	finishedJobState := j.wait()

	// update state
	r.m.Lock()
	status := r.statusStore[id]
	status.State = finishedJobState.state
	status.FinishedAt = &finishedJobState.finishedAt
	status.ExitCode = finishedJobState.exitCode
	status.TerminationSignal = finishedJobState.terminationSignal
	r.statusStore[id] = status
	r.m.Unlock()

	// Publish final status before readers receive EOF.
	o.finish()

	// allow Registry.StopJob to finish
	close(j.done)
}

func (r *Registry) JobStatus(id string) (Status, error) {
	r.m.RLock()
	defer r.m.RUnlock()
	status, ok := r.statusStore[id]
	if !ok {
		return Status{}, ErrJobNotFound
	}

	return status, nil
}

// StopJob issues a SIGKILL to a job. (TODO: implement graceful exit)
// If a job is already finished, ErrJobAlreadyStopped is returned.
func (r *Registry) StopJob(id string) (Status, error) {
	r.m.RLock()
	status, statusOk := r.statusStore[id]
	job, jobOk := r.jobStore[id]
	r.m.RUnlock()

	if !statusOk || !jobOk {
		return Status{}, ErrJobNotFound
	}

	// check if job is finished before issuing the signal
	if status.State != StateRunning {
		return Status{}, ErrJobAlreadyStopped
	}

	err := job.stop()
	// job may have finished in the time it took to send the signal
	if errors.Is(err, os.ErrProcessDone) {
		return Status{}, ErrJobAlreadyStopped
	}
	if err != nil {
		return Status{}, err
	}

	// wait for Registry.monitorJob to finish
	<-job.done

	// re-fetch status
	r.m.RLock()
	status = r.statusStore[id]
	r.m.RUnlock()

	return status, nil
}

// OpenStream replays a job's output from the beginning, then waits for new
// output until the job finishes. Closing the stream does not stop the job.
func (r *Registry) OpenStream(id string) (io.ReadCloser, error) {
	r.m.RLock()
	defer r.m.RUnlock()

	output, ok := r.outputStore[id]
	if !ok {
		return nil, ErrJobNotFound

	}

	return &outputReader{
		output:       output,
		readerClosed: make(chan struct{}),
	}, nil
}

// TODO: Add a ListJobs to list all the jobs
// TODO: Add shutdown support that rejects new jobs and applies
// and stops or waits for running jobs.
