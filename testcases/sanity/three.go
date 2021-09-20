package sanity

import (
	"time"

	"github.com/ds-test-framework/scheduler/log"
	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

type threeAction struct {
}

func (t *threeAction) handleFaulty(c *testlib.Context, message *types.Message, tMsg *util.TMessageWrapper) *types.Message {
	replica, _ := c.Replicas.Get(message.From)
	newVote, err := util.ChangeVote(replica, tMsg)
	if err != nil {
		return message
	}
	newMsgB, err := util.Marshal(newVote)
	if err != nil {
		return message
	}
	return c.NewMessage(message, newMsgB)
}

func (t *threeAction) handle0Prevote(c *testlib.Context, message *types.Message, tMsg *util.TMessageWrapper) []*types.Message {
	voteCount := getVoteCount(c)
	faults, _ := c.Vars.GetInt("faults")
	_, ok := voteCount[string(message.To)]
	if !ok {
		voteCount[string(message.To)] = 0
	}
	curCount := voteCount[string(message.To)]
	if curCount >= 2*faults-1 {
		return []*types.Message{}
	}
	voteCount[string(message.To)] = curCount + 1
	c.Vars.Set("voteCount", voteCount)
	return []*types.Message{message}
}

func (t *threeAction) handle0Propose(c *testlib.Context, message *types.Message, tMsg *util.TMessageWrapper) []*types.Message {
	blockID, ok := util.GetProposalBlockID(tMsg)
	if ok {
		c.Vars.Set("oldProposal", blockID)
	}
	return []*types.Message{message}
}

func (t *threeAction) Round0(c *testlib.Context) []*types.Message {
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

	_, round := util.ExtractHR(tMsg)
	if round == -1 {
		return []*types.Message{message}
	}
	if round != 0 {
		return []*types.Message{}
	}
	partition := getPartition(c)
	faulty, _ := partition.GetPart("faulty")
	toNotLock, _ := partition.GetPart("toNotLock")

	switch {
	case faulty.Contains(message.From) && (tMsg.Type == util.Precommit || tMsg.Type == util.Prevote):
		return []*types.Message{t.handleFaulty(c, message, tMsg)}
	case tMsg.Type == util.Prevote && round == 0 && toNotLock.Contains(message.To):
		return t.handle0Prevote(c, message, tMsg)
	case tMsg.Type == util.Proposal && round == 0:
		return t.handle0Propose(c, message, tMsg)
	}

	return []*types.Message{message}
}

func (t *threeAction) Round1(c *testlib.Context) []*types.Message {
	messages := make([]*types.Message, 0)
	if c.CurEvent.IsMessageSend() {
		messageID, _ := c.CurEvent.MessageID()
		message, ok := c.MessagePool.Get(messageID)
		if ok {
			messages = append(messages, message)
		}
	}

	parseOld, ok := c.Vars.GetBool("gatherRound1Messages")
	if !ok {
		c.Vars.Set("gatherRound1Messages", false)
		parseOld = false
	}
	if !parseOld {
		messages = append(messages, getRoundMessages(c, 1)...)
		c.Vars.Set("gatherRound1Messages", true)
	}

	result := make([]*types.Message, 0)
	for _, message := range messages {
		tMsg, err := util.Unmarshal(message.Data)
		if err != nil {
			result = append(result, message)
			continue
		}

		_, round := util.ExtractHR(tMsg)
		if round == -1 {
			result = append(result, message)
			continue
		}
		if round != 1 {
			continue
		}
		partition := getPartition(c)
		faulty, _ := partition.GetPart("faulty")

		if faulty.Contains(message.From) && (tMsg.Type == util.Precommit || tMsg.Type == util.Prevote) {
			result = append(result, t.handleFaulty(c, message, tMsg))
			continue
		}
		if tMsg.Type == util.Proposal {
			continue
		}

		result = append(result, message)
	}
	return result
}

func (t *threeAction) Round2(c *testlib.Context) []*types.Message {
	messages := make([]*types.Message, 0)
	if c.CurEvent.IsMessageSend() {
		messageID, _ := c.CurEvent.MessageID()
		message, ok := c.MessagePool.Get(messageID)
		if ok {
			messages = append(messages, message)
		}
	}

	parseOld, ok := c.Vars.GetBool("gatherRound2Messages")
	if !ok {
		c.Vars.Set("gatherRound2Messages", false)
		parseOld = false
	}
	if !parseOld {
		messages = append(messages, getRoundMessages(c, 2)...)
		c.Vars.Set("gatherRound2Messages", true)
	}

	result := make([]*types.Message, 0)
	for _, message := range messages {
		tMsg, err := util.Unmarshal(message.Data)
		if err != nil {
			result = append(result, message)
			continue
		}
		_, round := util.ExtractHR(tMsg)
		if round == -1 {
			result = append(result, message)
			continue
		}
		if round != 2 {
			continue
		}
		partition := getPartition(c)
		faulty, _ := partition.GetPart("faulty")
		if faulty.Contains(message.From) && (tMsg.Type == util.Precommit || tMsg.Type == util.Prevote) {
			result = append(result, t.handleFaulty(c, message, tMsg))
			continue
		}
		result = append(result, message)
	}
	return result
}

func threeSetup(c *testlib.Context) error {
	faults := int((c.Replicas.Cap() - 1) / 3)
	partitioner := util.NewGenericParititioner(c.Replicas)
	partition, err := partitioner.CreateParition([]int{faults + 1, 2*faults - 1, 1}, []string{"toLock", "toNotLock", "faulty"})
	if err != nil {
		return err
	}
	c.Vars.Set("partition", partition)
	c.Vars.Set("voteCount", make(map[types.ReplicaID]int))
	c.Vars.Set("faults", faults)
	c.Vars.Set("roundCount", make(map[string]int))
	return nil
}

func threeCommitCond(c *testlib.Context) bool {
	cEventType := c.CurEvent.Type
	switch cEventType := cEventType.(type) {
	case *types.GenericEventType:
		if cEventType.T != "Committing block" {
			return false
		}
		blockID, ok := cEventType.Params["block_id"]
		if !ok {
			return false
		}
		oldProposalI, ok := c.Vars.Get("oldProposal")
		if !ok {
			return false
		}
		oldProposal := oldProposalI.(string)
		c.Logger().With(log.LogParams{
			"old_proposal": oldProposal,
			"commit_block": blockID,
		}).Info("Checking commit")
		if blockID == oldProposal {
			return true
		}
	}
	return false
}

func ThreeTestCase() *testlib.TestCase {
	testcase := testlib.NewTestCase("LockedValueCheck", 30*time.Second)

	testcase.SetupFunc(threeSetup)

	action := &threeAction{}

	builder := testcase.Builder().Action(action.Round0)
	builder.On(commitCond, testlib.FailureStateLabel)

	round1State := builder.On(getRoundReachedCond(1), "round1").Action(action.Round1)
	round1State.On(commitCond, testlib.FailureStateLabel)

	round2State := round1State.On(getRoundReachedCond(2), "round2").Action(action.Round2)
	round2State.On(threeCommitCond, testlib.SuccessStateLabel)

	return testcase
}
