package lockedvalue

import (
	"strconv"
	"time"

	"github.com/ds-test-framework/scheduler/log"
	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

type oneAction struct {
}

func newOneAction() *oneAction {
	return &oneAction{}
}

func (o *oneAction) isFaultyVote(c *testlib.Context, tMsg *util.TMessageWrapper, message *types.Message) bool {
	partition := getPartition(c)
	faulty, _ := partition.GetPart("faulty")
	return (tMsg.Type == util.Prevote || tMsg.Type == util.Precommit) && faulty.Contains(message.From)
}

func (o *oneAction) changeFultyVote(c *testlib.Context, tMsg *util.TMessageWrapper, message *types.Message) *types.Message {
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

func (o *oneAction) Base(c *testlib.Context) (*types.Message, *util.TMessageWrapper, bool) {
	if !c.CurEvent.IsMessageSend() {
		return nil, nil, false
	}
	messageID, _ := c.CurEvent.MessageID()
	message, ok := c.MessagePool.Get(messageID)
	if !ok {
		return nil, nil, false
	}
	tMsg, err := util.Unmarshal(message.Data)
	if err != nil {
		return nil, nil, false
	}
	return message, tMsg, true
}

func (o *oneAction) HandleZero(c *testlib.Context) []*types.Message {
	message, tMsg, ok := o.Base(c)
	if !ok {
		return []*types.Message{}
	}

	if o.isFaultyVote(c, tMsg, message) {
		return []*types.Message{o.changeFultyVote(c, tMsg, message)}
	}

	_, round := util.ExtractHR(tMsg)
	if round == -1 {
		return []*types.Message{message}
	}
	if round != 0 {
		return []*types.Message{}
	}

	switch tMsg.Type {
	case util.Proposal:
		if blockID, ok := util.GetProposalBlockID(tMsg); ok {
			c.Vars.Set("oldProposal", blockID)
		}
	case util.Prevote:
		partition := getPartition(c)
		honestDelayed, _ := partition.GetPart("honestDelayed")

		if honestDelayed.Contains(message.From) {
			delayedPrevotesI, ok := c.Vars.Get("delayedPrevotes")
			if !ok {
				c.Vars.Set("delayedPrevotes", make([]string, 0))
				delayedPrevotesI, _ = c.Vars.Get("delayedPrevotes")
			}

			delayedPrevotes := delayedPrevotesI.([]string)
			delayedPrevotes = append(delayedPrevotes, message.ID)
			c.Vars.Set("delayedPrevotes", delayedPrevotes)
			return []*types.Message{}
		}
	}
	return []*types.Message{message}
}

// traverse the EventDAG to find messages that belong to a particular round and haven't been delivered yet
// The traversal stops when we hit a newRound event, this works because we traverse back from the latest event in each replica
func (o *oneAction) getRoundMessages(c *testlib.Context, round int) []*types.Message {
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

func (o *oneAction) HandleOne(c *testlib.Context) []*types.Message {

	messages := make([]*types.Message, 0)
	if c.CurEvent.IsMessageSend() {
		messageID, _ := c.CurEvent.MessageID()
		message, ok := c.MessagePool.Get(messageID)
		if ok {
			messages = append(messages, message)
		}
	}

	parseOld, ok := c.Vars.GetBool("gatherRound1Messages")
	if !ok || !parseOld {
		messages = append(messages, o.getRoundMessages(c, 1)...)
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

		if o.isFaultyVote(c, tMsg, message) {
			result = append(result, o.changeFultyVote(c, tMsg, message))
			continue
		}

		partition := getPartition(c)
		honestDelayed, _ := partition.GetPart("honestDelayed")

		if tMsg.Type == util.Proposal {
			delayedProposalI, ok := c.Vars.Get("delayedProposals")
			if !ok {
				c.Vars.Set("delayedProposals", make([]string, 0))
				delayedProposalI, _ = c.Vars.Get("delayedProposals")
			}
			delayedProposal := delayedProposalI.([]string)
			delayedProposal = append(delayedProposal, message.ID)
			c.Vars.Set("delayedProposals", delayedProposal)
			continue
		} else if tMsg.Type == util.Prevote && honestDelayed.Contains(message.From) {
			voteBlockID, ok := util.GetVoteBlockID(tMsg)
			if ok {
				oldProposal, _ := c.Vars.GetString("oldProposal")
				if voteBlockID != oldProposal {
					c.Logger().With(log.LogParams{
						"old_proposal": oldProposal,
						"vote_blockid": voteBlockID,
					}).Info("Failing because locked value was not voted")
					c.Fail()
				}
			}
		}

		result = append(result, message)
	}

	return result
}

func (o *oneAction) HandleTwo(c *testlib.Context) []*types.Message {

	messages := make([]*types.Message, 0)
	if c.CurEvent.IsMessageSend() {
		messageID, _ := c.CurEvent.MessageID()
		message, ok := c.MessagePool.Get(messageID)
		if ok {
			messages = append(messages, message)
		}
	}

	parseOld, ok := c.Vars.GetBool("gatherRound2Messages")
	if !ok || !parseOld {
		messages = append(messages, o.getRoundMessages(c, 2)...)
		c.Vars.Set("gatherRound2Messages", true)
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
		if round != 2 {
			continue
		}

		if o.isFaultyVote(c, tMsg, message) {
			result = append(result, o.changeFultyVote(c, tMsg, message))
			continue
		}

		switch tMsg.Type {
		case util.Proposal:
			blockID, ok := util.GetProposalBlockID(tMsg)
			if ok {
				c.Vars.Set("newProposal", blockID)
				oldPropI, ok := c.Vars.Get("oldProposal")
				if ok {
					oldProp := oldPropI.(string)
					if oldProp == blockID {
						c.Logger().With(log.LogParams{
							"round0_proposal": oldProp,
							"round2_proposal": blockID,
						}).Info("Failing because proposals are the same! Expecting different proposals")
						c.Fail()
					}
				}
			}
		case util.Prevote:
			partition := getPartition(c)
			honestDelayed, _ := partition.GetPart("honestDelayed")

			if honestDelayed.Contains(message.From) {
				oldPropI, _ := c.Vars.Get("oldProposal")
				newPropI, _ := c.Vars.Get("newProposal")
				oldProp := oldPropI.(string)
				newProp := newPropI.(string)
				voteBlockID, ok := util.GetVoteBlockID(tMsg)
				if ok && voteBlockID == oldProp {
					c.Logger().With(log.LogParams{
						"round0_proposal": oldProp,
						"vote":            voteBlockID,
					}).Info("Failing because replica did not unlock")
					c.Fail()
				} else if ok && voteBlockID == newProp {
					c.Success()
				}
			}
		}
		result = append(result, message)
	}

	return result
}

func onesetup(c *testlib.Context) error {
	faults := int((c.Replicas.Cap() - 1) / 3)
	partitioner := util.NewStaticPartitioner(c.Replicas, faults)
	partitioner.NewPartition(0)
	partition, _ := partitioner.GetPartition(0)
	c.Vars.Set("partition", partition)
	c.Vars.Set("faults", faults)
	c.Logger().With(log.LogParams{
		"partition": partition.String(),
	}).Info("Partitiion created")
	return nil
}

func getPartition(c *testlib.Context) *util.Partition {
	v, _ := c.Vars.Get("partition")
	return v.(*util.Partition)
}

func valueLockedCond(c *testlib.Context) bool {
	if !c.CurEvent.IsMessageReceive() {
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

	partition := getPartition(c)
	honestDelayed, _ := partition.GetPart("honestDelayed")

	c.Logger().With(log.LogParams{
		"message_id":    message.ID,
		"to":            message.To,
		"type":          tMsg.Type,
		"honestDelayed": honestDelayed.String(),
		"replica":       c.CurEvent.Replica,
	}).Debug("message receive event")

	if tMsg.Type == util.Prevote && honestDelayed.Contains(message.To) {
		c.Logger().With(log.LogParams{
			"message_id": message.ID,
		}).Debug("Prevote received by honest delayed")
		fI, _ := c.Vars.Get("faults")
		faults := fI.(int)

		voteBlockID, ok := util.GetVoteBlockID(tMsg)
		if ok {
			oldBlockID, ok := c.Vars.GetString("oldProposal")
			if ok && voteBlockID == oldBlockID {
				votes, ok := c.Vars.GetInt("prevotesSent")
				if !ok {
					votes = 0
				}
				votes++
				c.Vars.Set("prevotesSent", votes)
				if votes >= (2 * faults) {
					return true
				}
			}
		}
	}
	return false
}

func getRoundCond(toRound int) testlib.Condition {
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
		rI, ok := c.Vars.Get("roundCount")
		if !ok {
			c.Vars.Set("roundCount", map[string]int{})
			rI, _ = c.Vars.Get("roundCount")
		}
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

func commitNewCond(c *testlib.Context) bool {
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
		newProposalI, ok := c.Vars.Get("newProposal")
		if !ok {
			return false
		}
		newProposal := newProposalI.(string)
		c.Logger().With(log.LogParams{
			"new_proposal": newProposal,
			"commit_block": blockID,
		}).Info("Checking commit")
		if blockID == newProposal {
			return true
		}
	}
	return false
}

func commitOldCond(c *testlib.Context) bool {
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

func One() *testlib.TestCase {
	testcase := testlib.NewTestCase("LockedValueOne", 50*time.Second)
	testcase.SetupFunc(onesetup)

	oneAction := newOneAction()

	builder := testcase.Builder()

	round2 := builder.Action(oneAction.HandleZero).
		On(valueLockedCond, "LockedValue").Action(oneAction.HandleZero).
		On(getRoundCond(1), "Round1").Action(oneAction.HandleOne).
		On(getRoundCond(2), "Round2").Action(oneAction.HandleTwo)

	round2.On(commitNewCond, testlib.SuccessStateLabel)
	round2.On(commitOldCond, testlib.FailureStateLabel)

	return testcase
}
