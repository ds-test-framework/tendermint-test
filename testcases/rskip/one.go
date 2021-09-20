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
		if !c.CurEvent.IsMessageSend() {
			return false
		}
		messageID, _ := c.CurEvent.MessageID()
		message, ok := c.MessagePool.Get(messageID)
		if !ok {
			return false
		}
		tMsg, err := util.Unmarshal(message.Data)
		if err != nil {
			return false
		}
		mHeight, _ := util.ExtractHR(tMsg)
		return mHeight >= height
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

	if !c.CurEvent.IsMessageSend() {
		return false
	}
	messageID, _ := c.CurEvent.MessageID()
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

func getUndeliveredMessages(c *testlib.Context) []*types.Message {
	sentMessages := make(map[string]*types.Message)
	deliveredMessages := make(map[string]*types.Message)

	for _, replica := range c.Replicas.Iter() {
		latest, ok := c.EventDAG.GetLatestEvent(replica.ID)
		if !ok {
			continue
		}
		for latest != nil {
			switch latest.Event.Type.(type) {
			case *types.MessageSendEventType:
				messageID, _ := latest.Event.MessageID()
				message, ok := c.MessagePool.Get(messageID)
				if ok {
					sentMessages[messageID] = message
				}
			case *types.MessageReceiveEventType:
				messageID, _ := latest.Event.MessageID()
				message, ok := c.MessagePool.Get(messageID)
				if ok {
					deliveredMessages[messageID] = message
				}
			}
			latest = latest.GetPrev()
		}
	}
	result := make([]*types.Message, 0)
	for id := range deliveredMessages {
		delete(sentMessages, id)
	}
	for _, message := range sentMessages {
		result = append(result, message)
	}
	return result
}

func changeAndDelayVotesAction(c *testlib.Context) []*types.Message {

	messages := make([]*types.Message, 0)

	if c.CurEvent.IsMessageSend() {
		messageID, _ := c.CurEvent.MessageID()
		message, ok := c.MessagePool.Get(messageID)
		if ok {
			messages = append(messages, message)
		}
	}

	prevMessages, ok := c.Vars.GetBool("gatherMessages")
	if !ok || !prevMessages {
		messages = append(messages, getUndeliveredMessages(c)...)
		c.Logger().Info("Fetched undelivered messages")
		c.Vars.Set("gatherMessages", true)
	}

	result := make([]*types.Message, 0)
	for _, message := range messages {
		tMsg, err := util.Unmarshal(message.Data)
		if err != nil {
			continue
		}
		if tMsg.Type != util.Prevote {
			result = append(result, message)
			continue
		}

		partition := getPartition(c)
		rest, _ := partition.GetPart("rest")
		faulty, _ := partition.GetPart("faulty")
		if rest.Contains(message.From) {
			result = append(result, message)
			continue
		} else if faulty.Contains(message.From) {
			replica, ok := c.Replicas.Get(message.From)
			if !ok {
				continue
			}
			newvote, err := util.ChangeVote(replica, tMsg)
			if err != nil {
				continue
			}
			data, err := util.Marshal(newvote)
			if err != nil {
				continue
			}
			result = append(result, c.NewMessage(message, data))
			continue
		} else {
			delayedM := getDelayedMStore(c)
			delayedM.Add(message)
		}
	}
	return result
}

func setupFunc(c *testlib.Context) error {
	faults := int((c.Replicas.Cap() - 1) / 3)
	partitioner := util.NewStaticPartitioner(c.Replicas, faults)
	partitioner.NewPartition(0)
	partition, _ := partitioner.GetPartition(0)
	c.Vars.Set("partition", partition)
	delayedMessages := types.NewMessageStore()
	c.Vars.Set("delayedMessages", delayedMessages)
	return nil
}

func getPartition(c *testlib.Context) *util.Partition {
	v, _ := c.Vars.Get("partition")
	return v.(*util.Partition)
}

func getDelayedMStore(c *testlib.Context) *types.MessageStore {
	v, _ := c.Vars.Get("delayedMessages")
	return v.(*types.MessageStore)
}

func deliverDelayedAction(c *testlib.Context) []*types.Message {

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
	messages := getDelayedMessages(c)
	if c.CurEvent.IsMessageSend() {
		messageID, _ := c.CurEvent.MessageID()
		message, ok := c.MessagePool.Get(messageID)
		if ok {
			messages = append(messages, message)
		}
	}
	return messages
}

func noDelayedMessagesCond(c *testlib.Context) bool {
	delayedMessages := getDelayedMStore(c)
	return delayedMessages.Size() == 0
}

func allowSetupMessages(c *testlib.Context) []*types.Message {
	if !c.CurEvent.IsMessageSend() {
		return []*types.Message{}
	}
	messageID, _ := c.CurEvent.MessageID()
	message, ok := c.MessagePool.Get(messageID)
	if !ok {
		return []*types.Message{}
	}
	tMsg, err := util.Unmarshal(message.Data)
	if err != nil {
		return []*types.Message{message}
	}
	height, round := util.ExtractHR(tMsg)
	if height == -1 || round == -1 {
		return []*types.Message{message}
	}
	return []*types.Message{}
}

func OneTestcase(height, round int) *testlib.TestCase {
	testcase := testlib.NewTestCase("RoundSkipPrevote", 30*time.Second)
	testcase.SetupFunc(setupFunc)

	builder := testcase.Builder()

	builder.Action(allowSetupMessages).
		On(getHeightReachedCond(height), "delayAndChangeVotes").
		Action(changeAndDelayVotesAction).
		On(newRoundReachedCond(round).Check, "deliverDelayed").
		Action(deliverDelayedAction).
		On(noDelayedMessagesCond, testlib.SuccessStateLabel)

	return testcase
}
