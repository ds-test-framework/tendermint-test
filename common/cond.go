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

func IsMessageFromRound(round int) MessageCondition {
	return func(m *util.TMessage, c *testlib.Context) bool {
		return m.Round() == round
	}
}

func IsVoteFromPart(partS string) MessageCondition {
	return func(m *util.TMessage, c *testlib.Context) bool {
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

func IsVoteFromFaulty() MessageCondition {
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

func IsFromPart(partS string) MessageCondition {
	return func(m *util.TMessage, c *testlib.Context) bool {
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

func IsToPart(partS string) MessageCondition {
	return func(m *util.TMessage, c *testlib.Context) bool {
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

func IsMessageType(t util.MessageType) MessageCondition {
	return func(m *util.TMessage, c *testlib.Context) bool {
		return m.Type == t
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
		message, err := util.GetMessageFromEvent(e, c)
		if err != nil {
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

type MessageCondition func(*util.TMessage, *testlib.Context) bool

func (m MessageCondition) AsFunc() handlers.Condition {
	return func(e *types.Event, c *testlib.Context) bool {
		_, ok := e.MessageID()
		if !ok {
			return false
		}
		message, err := util.GetMessageFromEvent(e, c)
		if err != nil {
			return false
		}
		message.Event = e
		return m(message, c)
	}
}

func (m MessageCondition) And(other MessageCondition) MessageCondition {
	return func(t *util.TMessage, c *testlib.Context) bool {
		return m(t, c) && other(t, c)
	}
}

func (m MessageCondition) Or(other MessageCondition) MessageCondition {
	return func(t *util.TMessage, c *testlib.Context) bool {
		return m(t, c) || other(t, c)
	}
}

func (m MessageCondition) Not() MessageCondition {
	return func(t *util.TMessage, c *testlib.Context) bool {
		return !m(t, c)
	}
}
