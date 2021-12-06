package common

import (
	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/tendermint-test/util"
)

var (
	DefaultOptions = []SetupOption{addFN, partition}
)

type SetupOption func(*testlib.Context)

func Setup(options ...SetupOption) func(*testlib.Context) error {
	return func(c *testlib.Context) error {
		opts := append(DefaultOptions, options...)
		for _, o := range opts {
			o(c)
		}
		return nil
	}
}

func addFN(c *testlib.Context) {
	n := c.Replicas.Cap()
	f := int((n - 1) / 3)
	c.Vars.Set("n", n)
	c.Vars.Set("faults", f)
}

func partition(c *testlib.Context) {
	f := int((c.Replicas.Cap() - 1) / 3)
	partitioner := util.NewGenericPartitioner(c.Replicas)
	partition, _ := partitioner.CreatePartition(
		[]int{1, f, 2 * f},
		[]string{"h", "faulty", "rest"},
	)
	c.Vars.Set("partition", partition)
}
