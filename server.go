package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ds-test-framework/scheduler/config"
	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/tendermint-test/testcases/rskip"
	"github.com/ds-test-framework/tendermint-test/util"
)

func main() {

	termCh := make(chan os.Signal, 1)
	signal.Notify(termCh, os.Interrupt, syscall.SIGTERM)

	server, err := testlib.NewTestingServer(
		&config.Config{
			APIServerAddr: "192.168.1.8:7074",
			NumReplicas:   4,
			LogConfig: config.LogConfig{
				Path: "/tmp/tendermint/log/checker.log",
			},
		},
		&util.TMessageParser{},
		[]*testlib.TestCase{
			// testcases.DummyTestCase(),
			rskip.OneTestcase(1, 2),
			// rskip.BlockingTestcase(),
			// lockedvalue.One(),
			// lockedvalue.Two(),
			// lockedvalue.Three(),
			// sanity.OneTestCase(),
			// sanity.TwoTestCase(),
			// sanity.ThreeTestCase(),
			//sanity.HigherProp(),
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
