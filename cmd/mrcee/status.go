package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	v1 "github.com/tomasger/mrcee/gen/mrcee/v1"
	"github.com/urfave/cli/v3"
	"golang.org/x/sys/unix"
)

// JobStatus holds the job information displayed by the CLI.
type JobStatus struct {
	ID                string
	State             v1.JobState
	StartedAt         time.Time
	FinishedAt        *time.Time
	ExitCode          *int32
	TerminationSignal *int32
}

func statusCmd() *cli.Command {
	return &cli.Command{
		Name:         "status",
		Usage:        "check status of a running job",
		Action:       withClient(status),
		ArgValidator: validateJobIDArgs,
		ArgsUsage:    "<job-id>",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "job-id",
				Required: true,
			},
		},
	}
}

func status(ctx context.Context, c *cli.Command, client v1.JobServiceClient) error {
	job, err := fetchJobStatus(ctx, client, c.StringArg("job-id"))
	if err != nil {
		return err
	}

	_, err = fmt.Fprint(c.Writer, job)
	return err
}

func (status JobStatus) String() string {
	const timestampLayout = "2006-01-02 15:04:05.0000"
	var s strings.Builder
	line := func(label string, value any) {
		fmt.Fprintf(&s, "%-12s%v\n", label, value)
	}
	line("Job ID:", status.ID)
	line("State:", strings.TrimPrefix(status.State.String(), "JOB_STATE_"))
	line("Started:", status.StartedAt.Local().Format(timestampLayout))
	if status.FinishedAt != nil {
		line("Finished:", status.FinishedAt.Local().Format(timestampLayout))
		line("Duration:", formatDuration(status.FinishedAt.Sub(status.StartedAt)))
	}
	if status.ExitCode != nil {
		line("Exit Code:", *status.ExitCode)
	}
	if status.TerminationSignal != nil {
		line("Signal:", formatSignal(*status.TerminationSignal))
	}
	return s.String()
}

// formatDuration drops fractional units while retaining precision for short jobs.
func formatDuration(duration time.Duration) string {
	for _, unit := range []time.Duration{time.Second, time.Millisecond, time.Microsecond} {
		if duration >= unit {
			return duration.Truncate(unit).String()
		}
	}
	return duration.String()
}

func formatSignal(signal int32) string {
	if name := unix.SignalName(unix.Signal(signal)); name != "" {
		return fmt.Sprintf("%s (%d)", name, signal)
	}
	return fmt.Sprintf("%d", signal)
}

func fetchJobStatus(ctx context.Context, client v1.JobServiceClient, jobID string) (*JobStatus, error) {
	resp, err := client.GetJobStatus(ctx, &v1.GetJobStatusRequest{JobId: jobID})
	if err != nil {
		return nil, fmt.Errorf("cannot get status for job %q: %w", jobID, rpcError(err))
	}
	job := &JobStatus{
		ID:                resp.GetJobId(),
		State:             resp.GetState(),
		StartedAt:         resp.GetStartedAt().AsTime(),
		ExitCode:          resp.ExitCode,
		TerminationSignal: resp.TerminationSignal,
	}
	if resp.FinishedAt != nil {
		finished := resp.FinishedAt.AsTime()
		job.FinishedAt = &finished
	}
	return job, nil
}
