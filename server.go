package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ds-test-framework/scheduler/testing"
	"github.com/ds-test-framework/tendermint-test/testcases"
)

func main() {

	termCh := make(chan os.Signal, 1)
	signal.Notify(termCh, os.Interrupt, syscall.SIGTERM)

	server, err := testing.NewTestServer(
		testing.ServerConfig{
			Addr:     "192.168.0.5:7074",
			Replicas: 4,
			LogPath:  "/tmp/tendermint/log/checker.log",
		},
		[]*testing.TestCase{
			testcases.NewDummtTestCase(),
			// Parition strategy is to choose h, F (|F| = f) and R (|R|  = 2f) at random in the beginning
			// and to retain the same partition for further round skips
			// roundskip.NewRoundSkipPrevote(1, 2),
			// roundskip.NewRoundSkipBlockPart(1, 2),
			// roundskip.NewPreviousVote(1, 5),
			// roundskip.PrevoteTestCase(1, 4),
		},
	)

	if err != nil {
		fmt.Printf("Failed to start server: %s\n", err.Error())
		os.Exit(1)
	}

	go func() {
		<-termCh
		server.Stop()
	}()

	server.Run()

}
