package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ds-test-framework/scheduler/config"
	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/tendermint-test/testcases/lockedvalue"
)

func main() {

	termCh := make(chan os.Signal, 1)
	signal.Notify(termCh, os.Interrupt, syscall.SIGTERM)

	server, err := testlib.NewTestingServer(
		&config.Config{},
		[]*testlib.TestCase{
			// testcases.DummyTestCase(),
			// rskip.OneTestcase(1, 2),
			// lockedvalue.One(),
			lockedvalue.Two(),
			// bfttime.OneTestCase(),
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

	server.Start()

}
