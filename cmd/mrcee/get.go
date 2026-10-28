package main

import (
	"context"

	"github.com/urfave/cli/v3"
)

func getCmd() *cli.Command {
	return &cli.Command{
		Name:      "get",
		Usage:     "fetch output of a running job",
		Action:    getOutput,
		ArgsUsage: "<job-id>",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "job-id",
				Required: true,
			},
		},
	}
}

func getOutput(ctx context.Context, c *cli.Command) error {

	return nil
}
