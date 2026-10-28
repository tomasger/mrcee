package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

// validateJobIDArgs ensures 
func validateJobIDArgs(_ context.Context, c *cli.Command) error {
	if c.Args().Len() > 1 {
		return fmt.Errorf("%s accepts only one job ID; usage: mrcee %s <job-id>", c.Name, c.Name)
	}
	return nil
}
