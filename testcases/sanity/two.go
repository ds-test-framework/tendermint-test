package sanity

import (
	"strconv"
	"time"

	"github.com/ds-test-framework/scheduler/log"
	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

type twoAction struct{}

func (t *twoAction) Round0(c *testlib.Context) []*types.Message {

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
		return []*types.Message{}
	}
	_, round := util.ExtractHR(tMsg)
	if round == -1 {
		return []*types.Message{message}
	}
	if round != 0 {
		return []*types.Message{}
	}

	if isFaultyVote(c, tMsg, message) {
		return []*types.Message{changeFultyVote(c, tMsg, message)}
	}

	if tMsg.Type == util.Precommit {
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
	}

	return []*types.Message{message}
}

func (t *twoAction) Round1(c *testlib.Context) []*types.Message {
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
			continue
		}

		_, round := util.ExtractHR(tMsg)
		if round == -1 {
			result = append(result, message)
		}
		if round != 1 {
			continue
		}

		if isFaultyVote(c, tMsg, message) {
			result = append(result, changeFultyVote(c, tMsg, message))
			continue
		}

		if tMsg.Type == util.Proposal {
			replica, _ := c.Replicas.Get(message.From)
			newProp, err := util.ChangeProposalBlockID(replica, tMsg)
			if err != nil {
				c.Logger().With(log.LogParams{"error": err}).Error("Failed to change proposal")
				result = append(result, message)
				continue
			}
			newMsgB, err := util.Marshal(newProp)
			if err != nil {
				c.Logger().With(log.LogParams{"error": err}).Error("Failed to marshal changed proposal")
				result = append(result, message)
				continue
			}
			result = append(result, c.NewMessage(message, newMsgB))
			continue
		}
		result = append(result, message)
	}

	return result
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
	return message
}

// traverse the EventDAG to find messages that belong to a particular round and haven't been delivered yet
// The traversal stops when we hit a newRound event, this works because we traverse back from the latest event in each replica
func getRoundMessages(c *testlib.Context, round int) []*types.Message {
	sentMessages := make(map[string]*types.Message)
	deliveredMessage := make(map[string]*types.Message)
	for _, replica := range c.Replicas.Iter() {
		last, ok := c.EventDAG.GetLatestEvent(replica.ID)
		if !ok {
			continue
		}
	StrandLoop:
		for last != nil {
			switch eventType := last.Event.Type.(type) {
			case *types.GenericEventType:
				if eventType.T == "newRound" {
					roundS := eventType.Params["round"]
					r, err := strconv.Atoi(roundS)
					if err != nil {
						break StrandLoop
					}
					if r == round {
						break StrandLoop
					}
					break StrandLoop
				}
			case *types.MessageSendEventType:
				message, ok := c.MessagePool.Get(eventType.MessageID)
				if ok {
					tMsg, err := util.Unmarshal(message.Data)
					if err == nil {
						_, r := util.ExtractHR(tMsg)
						if r == round {
							sentMessages[message.ID] = message
						}
					}
				}
			case *types.MessageReceiveEventType:
				message, ok := c.MessagePool.Get(eventType.MessageID)
				if ok {
					tMsg, err := util.Unmarshal(message.Data)
					if err == nil {
						_, r := util.ExtractHR(tMsg)
						if r == round {
							deliveredMessage[message.ID] = message
						}
					}
				}
			}
			last = last.GetPrev()
		}
	}

	for id := range deliveredMessage {
		_, ok := sentMessages[id]
		if ok {
			delete(sentMessages, id)
		}
	}
	result := make([]*types.Message, len(sentMessages))
	i := 0
	for _, m := range sentMessages {
		result[i] = m
		i++
	}
	return result
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

func commitCond(c *testlib.Context) bool {
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

func getRoundReachedCond(toRound int) testlib.Condition {
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
	testcase := testlib.NewTestCase("WrongProposal", 30*time.Second)
	testcase.SetupFunc(twoSetup)
	action := &twoAction{}
	builder := testcase.Builder().Action(action.Round0)

	builder.On(commitCond, testlib.FailureStateLabel)
	round1State := builder.On(getRoundReachedCond(1), "round1").Action(action.Round1)

	round1State.On(commitCond, testlib.SuccessStateLabel)

	// Should not reach round 2 since replicas prevote locked block and reach consensus
	round1State.On(getRoundReachedCond(2), testlib.FailureStateLabel)

	return testcase
}
