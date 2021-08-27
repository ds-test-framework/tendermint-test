package rskip

import (
	"sync"
	"time"

	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

func getHeightReachedCond(height int) testlib.Condition {
	return func(c *testlib.Context) bool {
		cEventType := c.CurEvent.Type
		switch cEventType := cEventType.(type) {
		case *types.MessageSendEventType:
			messageID := cEventType.MessageID
			message, ok := c.MessagePool.Get(messageID)
			if !ok {
				return false
			}
			tMsg, err := util.Unmarshal(message.Data)
			if err != nil {
				return false
			}
			mHeight, _ := util.ExtractHR(tMsg)
			if mHeight >= height {
				return true
			}
		}
		return false
	}
}

type roundReachedCond struct {
	replicas map[types.ReplicaID]int
	lock     *sync.Mutex
	round    int
}

func newRoundReachedCond(round int) *roundReachedCond {
	return &roundReachedCond{
		replicas: make(map[types.ReplicaID]int),
		lock:     new(sync.Mutex),
		round:    round,
	}
}

func (r *roundReachedCond) Check(c *testlib.Context) bool {
	cEventType := c.CurEvent.Type
	switch cEventType := cEventType.(type) {
	case *types.MessageSendEventType:
		messageID := cEventType.MessageID
		message, ok := c.MessagePool.Get(messageID)
		if !ok {
			return false
		}
		tMsg, err := util.Unmarshal(message.Data)
		if err != nil {
			return false
		}
		_, round := util.ExtractHR(tMsg)

		r.lock.Lock()
		r.replicas[message.From] = round
		r.lock.Unlock()
	}
	threshold := int(c.Replicas.Cap() * 2 / 3)
	count := 0
	r.lock.Lock()
	for _, round := range r.replicas {
		if round >= r.round {
			count = count + 1
		}
	}
	r.lock.Unlock()
	return count >= threshold
}

func getChangeAndDelayVotesAction() testlib.StateAction {
	return func(c *testlib.Context) []*types.Message {
		// change votes and delay votes here!
		return []*types.Message{}
	}
}

func setupFunc(c *testlib.Context) error {
	faults := int((c.Replicas.Cap() - 1) / 3)
	partitioner := util.NewStaticPartitioner(c.Replicas, faults)
	partitioner.NewPartition(0)
	partition, _ := partitioner.GetPartition(0)
	c.Vars.Set("partition", partition)
	return nil
}

func getPartition(c *testlib.Context) *util.Partition {
	v, _ := c.Vars.Get("partition")
	return v.(*util.Partition)
}

func OneTestcase(height, round int) *testlib.TestCase {
	testcase := testlib.NewTestCase("RoundSkipPrevote", 30*time.Second)
	testcase.SetupFunc(setupFunc)

	builder := testcase.Builder()

	builder.
		On(getHeightReachedCond(height), "delayAndChangeVotes").
		Do(getChangeAndDelayVotesAction()).
		On(newRoundReachedCond(round).Check, testcase.Success().Label)

	return testcase
}
