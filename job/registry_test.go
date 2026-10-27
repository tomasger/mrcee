package job_test

import (
	"io"
	"path/filepath"
	"sync"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tomasger/mrcee/job"
)

// TestRegistryErrors checks edge cases when
// interacting with exposed Registry methods.
func TestRegistryErrors(t *testing.T) {
	r := job.NewRegistry()
	reader, err := r.OpenStream("missing")
	require.ErrorIs(t, err, job.ErrJobNotFound)
	require.Nil(t, reader)
	_, err = r.JobStatus("missing")
	require.ErrorIs(t, err, job.ErrJobNotFound)
	_, err = r.StopJob("missing")
	require.ErrorIs(t, err, job.ErrJobNotFound)

	id, err := r.StartJob("", nil)
	require.ErrorIs(t, err, job.ErrEmptyCommand)
	require.Empty(t, id)

	id, err = r.StartJob(filepath.Join(t.TempDir(), "missing"), nil)
	require.Error(t, err)
	require.Empty(t, id)

}

// TestRegistryJobCompletion tests for successful job
// completions (StateExited) and metadata being populated correctly.
func TestRegistryJobCompletion(t *testing.T) {
	for _, tc := range []struct {
		name     string
		command  string
		output   string
		exitCode int
	}{
		{name: "empty_success", command: "exit 0"},
		{name: "output_success", command: "echo hello", output: "hello\n"},
		{name: "nonzero_exit", command: "exit 7", exitCode: 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := job.NewRegistry()
			id, err := r.StartJob("/bin/sh", []string{"-c", tc.command})
			require.NoError(t, err)
			reader, err := r.OpenStream(id)
			require.NoError(t, err)
			t.Cleanup(func() { assert.NoError(t, reader.Close()) })
			output, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, tc.output, string(output))

			s, err := r.JobStatus(id)
			require.NoError(t, err)
			require.Equal(t, id, s.ID)
			require.Equal(t, "/bin/sh", s.Command)
			require.Equal(t, []string{"-c", tc.command}, s.Args)
			require.Equal(t, job.StateExited, s.State)
			require.NotNil(t, s.FinishedAt)
			require.False(t, s.FinishedAt.Before(s.StartedAt))
			require.NotNil(t, s.ExitCode)
			require.Equal(t, tc.exitCode, *s.ExitCode)
			require.Nil(t, s.TerminationSignal)

			_, err = r.StopJob(id)
			require.ErrorIs(t, err, job.ErrJobAlreadyStopped)
		})
	}
}

func TestRegistryStopJob(t *testing.T) {
	r := job.NewRegistry()
	id, err := r.StartJob("/bin/sleep", []string{"30"})
	require.NoError(t, err)
	t.Cleanup(func() { r.StopJob(id) })

	running, err := r.JobStatus(id)
	require.NoError(t, err)
	require.Equal(t, job.StateRunning, running.State)
	require.Nil(t, running.FinishedAt)

	stopped, err := r.StopJob(id)
	require.NoError(t, err)
	require.Equal(t, job.StateStopped, stopped.State)
	require.NotNil(t, stopped.FinishedAt)
	require.NotNil(t, stopped.TerminationSignal)
	require.Equal(t, syscall.SIGKILL, *stopped.TerminationSignal)
	require.Nil(t, stopped.ExitCode)

	stored, err := r.JobStatus(id)
	require.NoError(t, err)
	require.Equal(t, stopped, stored)
	_, err = r.StopJob(id)
	require.ErrorIs(t, err, job.ErrJobAlreadyStopped)
}

func TestRegistryConcurrentJobs(t *testing.T) {
	r := job.NewRegistry()
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			id, err := r.StartJob("echo", []string{"hello"})
			if !assert.NoError(t, err) {
				return
			}
			reader, err := r.OpenStream(id)
			if !assert.NoError(t, err) {
				return
			}
			defer reader.Close()
			data, err := io.ReadAll(reader)
			assert.NoError(t, err)
			assert.Equal(t, "hello\n", string(data))
			s, err := r.JobStatus(id)
			assert.NoError(t, err)
			assert.Equal(t, job.StateExited, s.State)
		})
	}
	wg.Wait()
}
