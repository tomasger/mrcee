package main

import (
	"context"

	"github.com/urfave/cli/v3"
)

func startCmd() *cli.Command {
	return &cli.Command{
		Name:   "start",
		Usage:  "start a new job",
		Action: start,
	}
}

func start(ctx context.Context, c *cli.Command) error {

	return nil
}
