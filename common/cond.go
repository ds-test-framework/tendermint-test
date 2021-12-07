package common

import (
	"fmt"

	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/scheduler/testlib/handlers"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

func IsCommit(e *types.Event, _ *testlib.Context) bool {
	eType, ok := e.Type.(*types.GenericEventType)
	return ok && eType.T == "Committing block"
}

func IsMessageFromRound(round int) handlers.Condition {
	return func(e *types.Event, c *testlib.Context) bool {
		m, ok := util.GetMessageFromEvent(e, c)
		if !ok {
			return false
		}
		return m.Round() == round
	}
}

func IsVoteFromPart(partS string) handlers.Condition {
	return func(e *types.Event, c *testlib.Context) bool {
		m, ok := util.GetMessageFromEvent(e, c)
		if !ok {
			return false
		}
		if m.Type != util.Precommit && m.Type != util.Prevote {
			return false
		}

		partition, ok := getPartition(c)
		if !ok {
			return false
		}
		part, ok := partition.GetPart(partS)
		if !ok {
			return false
		}
		val, ok := util.GetVoteValidator(m)
		if !ok {
			return false
		}
		return part.ContainsVal(val)
	}
}

func IsVoteFromFaulty() handlers.Condition {
	return IsVoteFromPart("faulty")
}

func getPartition(c *testlib.Context) (*util.Partition, bool) {
	p, exists := c.Vars.Get("partition")
	if !exists {
		return nil, false
	}
	partition, ok := p.(*util.Partition)
	return partition, ok
}

func IsFromPart(partS string) handlers.Condition {
	return func(e *types.Event, c *testlib.Context) bool {
		m, ok := util.GetMessageFromEvent(e, c)
		if !ok {
			return false
		}
		partition, ok := getPartition(c)
		if !ok {
			return false
		}
		part, ok := partition.GetPart(partS)
		if !ok {
			return false
		}
		return part.Contains(m.From)
	}
}

func IsToPart(partS string) handlers.Condition {
	return func(e *types.Event, c *testlib.Context) bool {
		m, ok := util.GetMessageFromEvent(e, c)
		if !ok {
			return false
		}
		partition, ok := getPartition(c)
		if !ok {
			return false
		}
		part, ok := partition.GetPart(partS)
		if !ok {
			return false
		}
		return part.Contains(m.To)
	}
}

func IsMessageType(t util.MessageType) handlers.Condition {
	return func(e *types.Event, c *testlib.Context) bool {
		message, ok := c.GetMessage(e)
		if !ok {
			return false
		}
		tMessage, ok := util.GetParsedMessage(message)
		if !ok {
			return false
		}
		return tMessage.Type == t
	}
}

func RoundReached(r int) handlers.Condition {
	return func(e *types.Event, c *testlib.Context) bool {
		roundKey := fmt.Sprintf("roundCount_%d", r)
		if !c.Vars.Exists(roundKey) {
			c.Vars.SetCounter(roundKey)
		}
		roundcounter, _ := c.Vars.GetCounter(roundKey)

		n, _ := c.Vars.GetInt("n")
		if roundcounter.Value() == n {
			return true
		}

		if !e.IsMessageSend() {
			return false
		}
		message, ok := util.GetMessageFromEvent(e, c)
		if !ok {
			return false
		}
		msgRound := message.Round()
		msgRoundKey := fmt.Sprintf("roundCount_%d", msgRound)
		replicaKey := fmt.Sprintf("roundCount_%s_%d", message.From, msgRound)
		if c.Vars.Exists(replicaKey) {
			return false
		}
		c.Vars.Set(replicaKey, true)
		if !c.Vars.Exists(msgRoundKey) {
			c.Vars.SetCounter(msgRoundKey)
		}
		msgRoundCounter, _ := c.Vars.GetCounter(msgRoundKey)
		msgRoundCounter.Incr()
		return roundcounter.Value() == n
	}
}
