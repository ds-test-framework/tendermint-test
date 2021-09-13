package lockedvalue

import (
	"fmt"
	"time"

	"github.com/ds-test-framework/scheduler/log"
	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/scheduler/types"
	sutil "github.com/ds-test-framework/scheduler/util"
	"github.com/ds-test-framework/tendermint-test/util"
)

func getAction() testlib.StateAction {
	return func(c *testlib.Context) []*types.Message {
		var message *types.Message
		message = nil
		cEventType := c.CurEvent.Type
		switch cEventType := cEventType.(type) {
		case *types.MessageSendEventType:
			messageID := cEventType.MessageID
			msg, ok := c.MessagePool.Get(messageID)
			if !ok {
				c.Logger().With(log.LogParams{
					"message_id": messageID,
				}).Info("Message not found")
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

		c.Logger().With(log.LogParams{
			"message_id": message.ID,
			"from":       message.From,
			"to":         message.To,
			"type":       tMsg.Type,
		}).Debug("Message was sent")

		switch tMsg.Type {
		case util.Proposal:
			_, round := util.ExtractHR(tMsg)
			if round == 1 {
				delayedProposalI, ok := c.Vars.Get("delayedProposals")
				if !ok {
					c.Vars.Set("delayedProposals", make([]string, 0))
					delayedProposalI, _ = c.Vars.Get("delayedProposals")
				}
				delayedProposal := delayedProposalI.([]string)
				delayedProposal = append(delayedProposal, message.ID)
				c.Vars.Set("delayedProposals", delayedProposal)
				return []*types.Message{}
			} else if round == 0 && !c.Vars.Exists("oldProposal") {
				blockID, ok := util.GetProposalBlockID(tMsg)
				if ok {
					c.Vars.Set("oldProposal", blockID)
				}
			} else if round == 2 && !c.Vars.Exists("newProposal") {
				blockID, ok := util.GetProposalBlockID(tMsg)
				if ok {
					c.Vars.Set("newProposal", blockID)
				}
			}
		case util.Prevote:
			_, round := util.ExtractHR(tMsg)

			partition := getPartition(c)
			honestDelayed, _ := partition.GetPart("honestDelayed")
			faulty, _ := partition.GetPart("faulty")

			if round == 0 && honestDelayed.Contains(message.From) {
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

			if faulty.Contains(message.From) {

				replica, _ := c.Replicas.Get(message.From)
				newVote, err := util.ChangeVote(replica, tMsg)
				if err != nil {
					return []*types.Message{message}
				}
				newMsgB, err := util.Marshal(newVote)
				if err != nil {
					return []*types.Message{message}
				}

				newMsg := message.Clone().(*types.Message)
				counter := getCounter(c)
				newMsg.ID = fmt.Sprintf("%s_%s_change%d", newMsg.From, newMsg.To, counter.Next())
				newMsg.Data = newMsgB
				return []*types.Message{newMsg}
			}
		default:
		}
		return []*types.Message{message}
	}
}

func getPartition(c *testlib.Context) *util.Partition {
	v, _ := c.Vars.Get("partition")
	return v.(*util.Partition)
}

func getCounter(c *testlib.Context) *sutil.Counter {
	v, _ := c.Vars.Get("counter")
	return v.(*sutil.Counter)
}

func valueLockedCond(c *testlib.Context) bool {
	cEventType := c.CurEvent.Type
	switch cEventType := cEventType.(type) {
	case *types.MessageReceiveEventType:
		messageID := cEventType.MessageID
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
			}).Info("Prevote received by honest delayed")
			fI, _ := c.Vars.Get("faults")
			faults := fI.(int)

			votesI, ok := c.Vars.Get("prevotesSent")
			if !ok {
				c.Vars.Set("prevotesSent", 0)
				votesI, _ = c.Vars.Get("prevotesSent")
			}
			votes := votesI.(int)
			votes++
			c.Vars.Set("prevotesSent", votes)
			if votes >= (2 * faults) {
				return true
			}
		}
	}
	return false
}

func getRoundCond(toRound int) testlib.Condition {
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
			if skipped == c.Replicas.Cap() {
				return true
			}
		}
		return false
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

func setup(c *testlib.Context) error {
	faults := int((c.Replicas.Cap() - 1) / 3)
	partitioner := util.NewStaticPartitioner(c.Replicas, faults)
	partitioner.NewPartition(0)
	partition, _ := partitioner.GetPartition(0)
	c.Vars.Set("partition", partition)
	c.Vars.Set("faults", faults)
	c.Vars.Set("counter", sutil.NewCounter())
	c.Logger().With(log.LogParams{
		"partition": partition.String(),
	}).Info("Partitiion created")
	return nil
}

func One() *testlib.TestCase {
	testcase := testlib.NewTestCase("LockedValueOne", 50*time.Second)
	testcase.SetupFunc(setup)

	testcase.Start().Action = getAction()

	builder := testcase.Builder()

	round2 := builder.
		On(valueLockedCond, "LockedValue").Action(getAction()).
		On(getRoundCond(1), "Round1").Action(getAction()).
		On(getRoundCond(2), "Round2").Action(getAction())

	round2.On(commitNewCond, testcase.Success().Label)
	round2.On(commitOldCond, testcase.Fail().Label)

	return testcase
}
