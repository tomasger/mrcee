package main

import (
	"flag"
	"log"

	"github.com/tomasger/mrcee/internal/server"
	"github.com/tomasger/mrcee/job"
)

var (
	address = flag.String("address", "127.0.0.1:10234", "ip:port to bind the server to")
)

func main() {
	flag.Parse()

	registry := job.NewRegistry()
	srv := server.NewServer(*address, registry)

	// TODO: Handle termination signals and gracefully shut down the server and job registry.
	log.Printf("Starting gRPC server on %s", *address)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("gRPC server failed: %v", err)
	}
}
