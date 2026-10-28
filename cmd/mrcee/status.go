package main

import (
	"context"

	"github.com/urfave/cli/v3"
)

func statusCmd() *cli.Command {
	return &cli.Command{
		Name:      "status",
		Usage:     "check status of a running job",
		Action:    status,
		ArgsUsage: "<job-id>",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "job-id",
				Required: true,
			},
		},
	}
}

func status(ctx context.Context, c *cli.Command) error {

	return nil
}
