package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name: "mrcee",
		Commands: []*cli.Command{
			startCmd(),
			statusCmd(),
			getCmd(),
			stopCmd(),
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "server",
				Aliases: []string{"s"},
				Value:   "127.0.0.1:10234",
			},
			&cli.StringFlag{
				Name:  "ca",
				Usage: "CA certificate file location",
				Value: ".local/certs/ca.crt",
			},
			&cli.StringFlag{
				Name:  "cert",
				Usage: "Client certificate file location",
				Value: ".local/certs/client.crt",
			},
			&cli.StringFlag{
				Name:  "key",
				Usage: "Client private key file location",
				Value: ".local/certs/client.key",
			},
		},
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := cmd.Run(ctx, os.Args)
	// Release signal resources before os.Exit, which skips deferred calls.
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "mrcee:", err)
		os.Exit(1)
	}
}
