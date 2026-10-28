package main

import (
	"context"

	"github.com/urfave/cli/v3"
)

func stopCmd() *cli.Command {
	return &cli.Command{
		Name:      "stop",
		Usage:     "stop a running job",
		Action:    stopJob,
		ArgsUsage: "<job-id>",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "job-id",
				Required: true,
			},
		},
	}
}

func stopJob(ctx context.Context, c *cli.Command) error {

	return nil
}
