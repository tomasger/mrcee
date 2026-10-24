package job

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJobStreamsProcessOutput(t *testing.T) {
	o := &output{changed: make(chan struct{})}
	j, err := startJob("/bin/sh", []string{"-c", "echo out; echo err >&2"}, o)
	require.NoError(t, err)

	j.wait()

	require.Equal(t, "out\nerr\n", string(o.data))
}

func TestJobSuccessfulExit(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command string
		args    []string
		output  string
	}{
		{
			name:    "with_output",
			command: "echo",
			args:    []string{"hello"},
			output:  "hello\n",
		},
		{
			name:    "without_output",
			command: "true",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := &output{changed: make(chan struct{})}
			job, err := startJob(tc.command, tc.args, o)
			require.NoError(t, err)

			finished := job.wait()
			require.Equal(t, tc.output, string(o.data))
			require.Equal(t, StateExited, finished.state)
			require.False(t, finished.finishedAt.IsZero())
			require.NotNil(t, finished.exitCode)
			require.Zero(t, *finished.exitCode)
			require.Nil(t, finished.terminationSignal)
		})
	}
}

func TestJobStop(t *testing.T) {
	o := &output{changed: make(chan struct{})}
	job, err := startJob("/bin/sleep", []string{"30"}, o)
	require.NoError(t, err)
	t.Cleanup(func() { job.cmd.Process.Signal(syscall.SIGKILL) })
	require.NoError(t, job.stop())
	finished := job.wait()
	require.Equal(t, StateStopped, finished.state)
	require.False(t, finished.finishedAt.IsZero())
	require.NotNil(t, finished.terminationSignal)
	require.Equal(t, syscall.SIGKILL, *finished.terminationSignal)
	require.Nil(t, finished.exitCode)
}

func TestJobAlreadyStopped(t *testing.T) {
	o := &output{changed: make(chan struct{})}
	j, err := startJob("true", nil, o)
	require.NoError(t, err)
	finished := j.wait()
	require.Equal(t, StateExited, finished.state)
	require.ErrorIs(t, j.stop(), os.ErrProcessDone)
}
