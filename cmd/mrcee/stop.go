package main

import (
	"context"
	"fmt"

	v1 "github.com/tomasger/mrcee/gen/mrcee/v1"
	"github.com/urfave/cli/v3"
)

func stopCmd() *cli.Command {
	return &cli.Command{
		Name:         "stop",
		Usage:        "stop a running job",
		Action:       withClient(stopJob),
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

func stopJob(ctx context.Context, c *cli.Command, client v1.JobServiceClient) error {
	jobID := c.StringArg("job-id")
	req := &v1.StopJobRequest{JobId: jobID}
	resp, err := client.StopJob(ctx, req)
	if err != nil {
		return fmt.Errorf("cannot stop job %q: %w", jobID, rpcError(err))
	}
	if resp.TerminationSignal != nil {
		_, err = fmt.Fprintf(c.Writer, "Terminated %s with signal %s\n", jobID, formatSignal(resp.GetTerminationSignal()))
		return err
	}
	return nil
}
