package job

import (
	"os/exec"
	"syscall"
	"time"
)

type State string

const (
	StateRunning State = "running"
	StateExited  State = "exited"
	StateStopped State = "stopped"
)

// job wraps a process and manages its lifecycle.
type job struct {
	cmd  *exec.Cmd
	done chan struct{}
}

// finishedJobState records when and how a process finished.
type finishedJobState struct {
	state             State
	finishedAt        time.Time
	terminationSignal *syscall.Signal
	exitCode          *int
}

// startJob starts the process with the supplied output buffer.
// It does not wait for the job to complete.
func startJob(command string, args []string, o *output) (*job, error) {
	// TODO: Accept execution options for working directory and environment
	// instead of always inheriting the worker process's settings.
	j := &job{
		cmd:  exec.Command(command, args...),
		done: make(chan struct{}),
	}

	j.cmd.Stdout = o
	j.cmd.Stderr = o

	if err := j.cmd.Start(); err != nil {
		return nil, err
	}

	return j, nil
}

// wait waits until process stops and shares its final state:
// * finished timestamp
// * whether process exited itself or was stopped
func (j *job) wait() finishedJobState {
	// TODO: handle error returned by cmd.Wait().
	// It can contain:
	// - an ExitError from non-zero exit code or
	//   a signal. We expose that via cmd.ProcessState instead.
	// - errors from io.Writer. This  does not occur in our
	//   implementation because output.Write() always succeeds.
	// as such it won't cause being left unhandled, but any other output implementation
	// e.g. storing output in files, would require this to be handled.
	j.cmd.Wait()
	finishedAt := time.Now()
	finishedState := finishedJobState{
		finishedAt: finishedAt,
	}

	if waitStatus, ok := j.cmd.ProcessState.Sys().(syscall.WaitStatus); ok && waitStatus.Signaled() {
		signal := waitStatus.Signal()
		finishedState.terminationSignal = &signal
		finishedState.state = StateStopped
	} else {
		exitCode := j.cmd.ProcessState.ExitCode()
		finishedState.exitCode = &exitCode
		finishedState.state = StateExited
	}

	return finishedState
}

// stop issues a SIGKILL for a job
func (j *job) stop() error {
	// TODO: implement graceful shutdown using SIGTERM
	// with a grace period, followed by SIGKILL.
	err := j.cmd.Process.Signal(syscall.SIGKILL)
	if err != nil {
		return err
	}

	return nil
}
