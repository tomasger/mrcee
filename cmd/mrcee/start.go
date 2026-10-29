package main

import (
	"context"
	"errors"
	"fmt"

	v1 "github.com/tomasger/mrcee/gen/mrcee/v1"
	"github.com/urfave/cli/v3"
)

func startCmd() *cli.Command {
	return &cli.Command{
		Name:   "start",
		Usage:  "start a new job",
		Action: withClient(start),
	}
}

func start(ctx context.Context, c *cli.Command, client v1.JobServiceClient) error {
	cliArgs := c.Args()
	if cliArgs.Len() == 0 {
		return errors.New("starting a job requires a command")
	}
	command := cliArgs.First()
	args := cliArgs.Slice()[1:]

	req := &v1.StartJobRequest{
		Executable: command,
		Args:       args,
	}
	resp, err := client.StartJob(ctx, req)
	if err != nil {
		return fmt.Errorf("cannot start %q: %w", command, rpcError(err))
	}

	_, err = fmt.Fprintln(c.Writer, "job started:", resp.GetJobId())
	return err
}
