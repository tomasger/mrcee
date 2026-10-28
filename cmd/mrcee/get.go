package main

import (
	"context"
	"fmt"
	"io"

	v1 "github.com/tomasger/mrcee/gen/mrcee/v1"
	"github.com/urfave/cli/v3"
)

func getCmd() *cli.Command {
	return &cli.Command{
		Name:         "get",
		Usage:        "fetch output of a running job",
		Action:       withClient(getOutput),
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

func getOutput(ctx context.Context, c *cli.Command, client v1.JobServiceClient) error {
	jobID := c.StringArg("job-id")
	operation := fmt.Sprintf("cannot get output for job %q", jobID)
	req := &v1.StreamOutputRequest{JobId: jobID}
	stream, err := client.StreamOutput(ctx, req)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, rpcError(err))
	}
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("%s: %w", operation, rpcError(err))
		}
		if _, err := c.Writer.Write(resp.GetData()); err != nil {
			return fmt.Errorf("cannot write job output: %w", err)
		}
	}
	return nil
}
