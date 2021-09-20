package rskip

import (
	"fmt"
	"sync"
	"time"

	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/scheduler/types"
	sutil "github.com/ds-test-framework/scheduler/util"
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
		curEventType := c.CurEvent.Type

		var message *types.Message
		message = nil

		switch curEventType := curEventType.(type) {
		case *types.MessageSendEventType:
			messageID := curEventType.MessageID
			msg, ok := c.MessagePool.Get(messageID)
			if !ok {
				return []*types.Message{}
			}
			message = msg
		}
		if message == nil {
			return []*types.Message{}
		}

		tMsg, err := util.Unmarshal(message.Data)
		if err != nil {
			return []*types.Message{}
		}
		if tMsg.Type != util.Prevote {
			return []*types.Message{message}
		}

		partition := getPartition(c)
		rest, _ := partition.GetPart("rest")
		faulty, _ := partition.GetPart("faulty")
		if rest.Contains(message.From) {
			return []*types.Message{message}
		} else if faulty.Contains(message.From) {
			replica, ok := c.Replicas.Get(message.From)
			if !ok {
				return []*types.Message{}
			}
			newvote, err := util.ChangeVote(replica, tMsg)
			if err != nil {
				return []*types.Message{}
			}
			data, err := util.Marshal(newvote)
			if err != nil {
				return []*types.Message{}
			}
			newMsg := message.Clone().(*types.Message)
			counter := getCounter(c)
			newMsg.ID = fmt.Sprintf("%s_%s_change%d", newMsg.From, newMsg.To, counter.Next())
			newMsg.Data = data
			return []*types.Message{newMsg}
		} else {
			delayedM := getDelayedMStore(c)
			delayedM.Add(message)
		}

		return []*types.Message{}
	}
}

func setupFunc(c *testlib.Context) error {
	faults := int((c.Replicas.Cap() - 1) / 3)
	partitioner := util.NewStaticPartitioner(c.Replicas, faults)
	partitioner.NewPartition(0)
	partition, _ := partitioner.GetPartition(0)
	c.Vars.Set("partition", partition)
	delayedMessages := types.NewMessageStore()
	c.Vars.Set("delayedMessages", delayedMessages)
	c.Vars.Set("counter", sutil.NewCounter())
	return nil
}

func getPartition(c *testlib.Context) *util.Partition {
	v, _ := c.Vars.Get("partition")
	return v.(*util.Partition)
}

func getCounter(c *testlib.Context) *sutil.Counter {
	v, _ := c.Vars.Get("counter")
	return v.(*sutil.Counter)
}

func getDelayedMStore(c *testlib.Context) *types.MessageStore {
	v, _ := c.Vars.Get("delayedMessages")
	return v.(*types.MessageStore)
}

func deliverDelayedAction(testcase *testlib.TestCase) testlib.StateAction {

	getDelayedMessages := func(c *testlib.Context) []*types.Message {
		delayedM := getDelayedMStore(c)
		if delayedM.Size() == 0 {
			return []*types.Message{}
		}
		messages := make([]*types.Message, delayedM.Size())
		for i, m := range delayedM.Iter() {
			messages[i] = m
			delayedM.Remove(m.ID)
		}
		c.Transition(testlib.SuccessStateLabel)
		return messages
	}

	return func(c *testlib.Context) []*types.Message {
		cEventType := c.CurEvent.Type
		messages := getDelayedMessages(c)
		switch cEventType := cEventType.(type) {
		case *types.MessageSendEventType:
			mID := cEventType.MessageID
			message, ok := c.MessagePool.Get(mID)
			if ok {
				messages = append(messages, message)
			}
		}
		return messages
	}
}

func getNoDelayedMessagesCond() testlib.Condition {
	return func(c *testlib.Context) bool {
		delayedMessages := getDelayedMStore(c)
		return delayedMessages.Size() == 0
	}
}

func OneTestcase(height, round int) *testlib.TestCase {
	testcase := testlib.NewTestCase("RoundSkipPrevote", 30*time.Second)
	testcase.SetupFunc(setupFunc)

	builder := testcase.Builder()

	builder.Action(testlib.AllowAllAction).
		On(getHeightReachedCond(height), "delayAndChangeVotes").
		Action(getChangeAndDelayVotesAction()).
		On(newRoundReachedCond(round).Check, "deliverDelayed").
		Action(deliverDelayedAction(testcase)).
		On(getNoDelayedMessagesCond(), testlib.SuccessStateLabel)

	return testcase
}
