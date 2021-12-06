package lockedvalue

import (
	"github.com/ds-test-framework/scheduler/log"
	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/scheduler/testlib/handlers"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

type commonCond struct{}

func (commonCond) updateRoundCount(c *testlib.Context, round int, from types.ReplicaID) map[string]int {
	rI, ok := c.Vars.Get("roundCount")
	if !ok {
		c.Vars.Set("roundCount", map[string]int{})
		rI, _ = c.Vars.Get("roundCount")
	}
	roundCount := rI.(map[string]int)
	cRound, ok := roundCount[string(from)]
	if !ok {
		roundCount[string(from)] = round
	}
	if cRound < round {
		roundCount[string(from)] = round
	}
	c.Vars.Set("roundCount", roundCount)
	return roundCount
}

func (t commonCond) roundReached(toRound int) handlers.Condition {
	return func(e *types.Event, c *testlib.Context) bool {
		tMsg, ok := util.GetMessageFromEvent(e, c)
		if !ok {
			return false
		}
		round := tMsg.Round()
		roundCount := t.updateRoundCount(c, round, tMsg.From)

		skipped := 0
		for _, r := range roundCount {
			if r >= toRound {
				skipped++
			}
		}
		if skipped == c.Replicas.Cap() {
			c.Logger().With(log.LogParams{"round": toRound}).Info("Reached round")
			return true
		}
		return false
	}
}

func (commonCond) valueLockedCond(e *types.Event, c *testlib.Context) bool {
	if !e.IsMessageReceive() {
		return false
	}
	message, _ := c.GetMessage(e)
	tMsg, ok := util.GetParsedMessage(message)
	if !ok {
		return false
	}
	partition := getReplicaPartition(c)
	honestDelayed, _ := partition.GetPart("honestDelayed")

	if tMsg.Type == util.Prevote && honestDelayed.Contains(tMsg.To) {
		c.Logger().With(log.LogParams{
			"message_id": message.ID,
		}).Debug("Prevote received by honest delayed")
		fI, _ := c.Vars.Get("faults")
		faults := fI.(int)

		voteBlockID, ok := util.GetVoteBlockIDS(tMsg)
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
					c.Logger().Info("2f+1 votes received! Value locked!")
					return true
				}
			}
		}
	}
	return false
}

type commonUtil struct{}

func (commonUtil) isFaultyVote(e *types.Event, c *testlib.Context, tMsg *util.TMessage) bool {
	partition := getReplicaPartition(c)
	faulty, _ := partition.GetPart("faulty")
	return (tMsg.Type == util.Prevote || tMsg.Type == util.Precommit) && faulty.Contains(tMsg.From)
}

func (commonUtil) changeFultyVote(e *types.Event, c *testlib.Context, message *types.Message, tMsg *util.TMessage) *types.Message {
	partition := getReplicaPartition(c)
	faulty, _ := partition.GetPart("faulty")
	if (tMsg.Type == util.Prevote || tMsg.Type == util.Precommit) && faulty.Contains(tMsg.From) {
		replica, _ := c.Replicas.Get(tMsg.From)
		newVote, err := util.ChangeVoteToNil(replica, tMsg)
		if err != nil {
			return message
		}
		newMsgB, err := newVote.Marshal()
		if err != nil {
			return message
		}

		return c.NewMessage(message, newMsgB)
	}
	return message
}
