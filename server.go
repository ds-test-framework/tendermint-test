package main

import (
	"fmt"
	"os"

	"github.com/ds-test-framework/scheduler/testing"
	"github.com/ds-test-framework/tendermint-test/testcases"
)

func main() {
	server, err := testing.NewTestServer(
		testing.ServerConfig{
			Addr:     "192.168.0.102:7074",
			Replicas: 4,
		},
		[]testing.TestCase{
			testcases.NewRoundSkipTest(),
		},
	)

	if err != nil {
		fmt.Printf("Failed to start server: %s\n", err.Error())
		os.Exit(1)
	}
	server.Run()
}
