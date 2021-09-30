package sanity

import (
	"time"

	"github.com/ds-test-framework/scheduler/log"
	"github.com/ds-test-framework/scheduler/testlib"
	smlib "github.com/ds-test-framework/scheduler/testlib/statemachine"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

type twoAction struct{}

func faultyFilter(c *smlib.Context) ([]*types.Message, bool) {
	if !c.CurEvent.IsMessageSend() {
		return []*types.Message{}, false
	}
	messageID, _ := c.CurEvent.MessageID()
	message, ok := c.MessagePool.Get(messageID)
	if !ok {
		return []*types.Message{}, false
	}

	tMsg, err := util.Unmarshal(message.Data)
	if err != nil {
		return []*types.Message{}, false
	}
	if isFaultyVote(c.Context, tMsg, message) {
		return []*types.Message{changeFultyVote(c.Context, tMsg, message)}, true
	}
	return []*types.Message{}, false
}

func round0_Two(c *smlib.Context) ([]*types.Message, bool) {

	if !c.CurEvent.IsMessageSend() {
		return []*types.Message{}, false
	}
	messageID, _ := c.CurEvent.MessageID()
	message, ok := c.MessagePool.Get(messageID)
	if !ok {
		return []*types.Message{}, false
	}

	tMsg, err := util.Unmarshal(message.Data)
	if err != nil {
		return []*types.Message{}, false
	}
	_, round := util.ExtractHR(tMsg)
	if round == -1 {
		return []*types.Message{message}, true
	}
	if round != 0 {
		return []*types.Message{}, false
	}

	if tMsg.Type == util.Precommit {
		voteCount := getVoteCount(c.Context)
		faults, _ := c.Vars.GetInt("faults")
		_, ok := voteCount[string(message.To)]
		if !ok {
			voteCount[string(message.To)] = 0
		}
		curCount := voteCount[string(message.To)]
		if curCount >= 2*faults-1 {
			return []*types.Message{}, true
		}
		voteCount[string(message.To)] = curCount + 1
		c.Vars.Set("voteCount", voteCount)
	}

	return []*types.Message{message}, true
}

func round1_Two(c *smlib.Context) ([]*types.Message, bool) {

	if !c.CurEvent.IsMessageSend() {
		return []*types.Message{}, false
	}
	messageID, _ := c.CurEvent.MessageID()
	message, ok := c.MessagePool.Get(messageID)
	if !ok {
		return []*types.Message{}, false
	}

	tMsg, err := util.Unmarshal(message.Data)
	if err != nil {
		return []*types.Message{}, false
	}

	_, round := util.ExtractHR(tMsg)
	if round == -1 {
		return []*types.Message{message}, true
	}
	if round != 1 {
		return []*types.Message{}, false
	}

	if tMsg.Type == util.Proposal {
		replica, _ := c.Replicas.Get(message.From)
		newProp, err := util.ChangeProposalBlockID(replica, tMsg)
		if err != nil {
			c.Logger().With(log.LogParams{"error": err}).Error("Failed to change proposal")
			return []*types.Message{message}, true
		}
		newMsgB, err := util.Marshal(newProp)
		if err != nil {
			c.Logger().With(log.LogParams{"error": err}).Error("Failed to marshal changed proposal")
			return []*types.Message{message}, true
		}
		return []*types.Message{c.NewMessage(message, newMsgB)}, true
	}
	return []*types.Message{message}, true
}

func isFaultyVote(c *testlib.Context, tMsg *util.TMessageWrapper, message *types.Message) bool {
	partition := getPartition(c)
	faulty, _ := partition.GetPart("faulty")
	return (tMsg.Type == util.Prevote || tMsg.Type == util.Precommit) && faulty.Contains(message.From)
}

func changeFultyVote(c *testlib.Context, tMsg *util.TMessageWrapper, message *types.Message) *types.Message {
	partition := getPartition(c)
	faulty, _ := partition.GetPart("faulty")
	if (tMsg.Type == util.Prevote || tMsg.Type == util.Precommit) && faulty.Contains(message.From) {
		replica, _ := c.Replicas.Get(message.From)
		newVote, err := util.ChangeVoteToNil(replica, tMsg)
		if err != nil {
			return message
		}
		newMsgB, err := util.Marshal(newVote)
		if err != nil {
			return message
		}
		return c.NewMessage(message, newMsgB)
	}
	return message
}

func getVoteCount(c *testlib.Context) map[string]int {
	v, _ := c.Vars.Get("voteCount")
	return v.(map[string]int)
}

func twoSetup(c *testlib.Context) error {
	faults := int((c.Replicas.Cap() - 1) / 3)
	partitioner := util.NewStaticPartitioner(c.Replicas, faults)
	partitioner.NewPartition(0)
	partition, _ := partitioner.GetPartition(0)
	c.Vars.Set("partition", partition)
	c.Vars.Set("faults", faults)
	c.Vars.Set("voteCount", make(map[string]int))
	c.Vars.Set("roundCount", make(map[string]int))
	return nil
}

func commitCond(c *smlib.Context) bool {
	cEventType := c.CurEvent.Type
	switch cEventType := cEventType.(type) {
	case *types.GenericEventType:
		if cEventType.T != "Committing block" {
			return false
		}
		return true
	}
	return false
}

func roundReached(toRound int) smlib.Condition {
	return func(c *smlib.Context) bool {
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
		rI, _ := c.Vars.Get("roundCount")
		roundCount := rI.(map[string]int)
		cRound, ok := roundCount[string(message.From)]
		if !ok {
			roundCount[string(message.From)] = round
		}
		if cRound < round {
			roundCount[string(message.From)] = round
		}
		c.Vars.Set("roundCount", roundCount)

		skipped := 0
		for _, r := range roundCount {
			if r >= toRound {
				skipped++
			}
		}
		return skipped == c.Replicas.Cap()
	}
}

// States:
// 	1. Ensure replicas skip round by not delivering enough precommits
//		1.1 One replica prevotes and precommits nil
// 	2. In the next round change the proposal block value
// 	3. Replicas should prevote and precommit nil and hence round skip
func TwoTestCase() *testlib.TestCase {

	sm := smlib.NewStateMachine()
	start := sm.Builder()
	start.On(commitCond, smlib.FailStateLabel)
	round1 := start.On(roundReached(1), "round1")
	round1.On(commitCond, smlib.SuccessStateLabel)
	round1.On(roundReached(2), smlib.FailStateLabel)

	handler := smlib.NewAsyncStateMachineHandler(sm)
	handler.AddEventHandler(faultyFilter)
	handler.AddEventHandler(round0_Two)
	handler.AddEventHandler(round1_Two)

	testcase := testlib.NewTestCase("WrongProposal", 30*time.Second, handler)
	testcase.SetupFunc(twoSetup)

	return testcase
}
