package lockedvalue

import (
	"errors"
	"sync"
	"time"

	"github.com/ds-test-framework/scheduler/log"
	"github.com/ds-test-framework/scheduler/testing"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/testcases/roundskip"
	"github.com/ds-test-framework/tendermint-test/util"
)

type LockedValue struct {
	testing.BaseTestCase

	allReplica  *types.ReplicaStore
	partitioner roundskip.Partitioner
	mtx         *sync.Mutex
	ready       chan bool
	allok       bool
	zerostate   *zerostate
	onestate    *onestate
	twostate    *twostate
}

func NewLockedValue() *LockedValue {
	return &LockedValue{
		BaseTestCase: *testing.NewBaseTestCase("OneLockedValue", 30*time.Second),
		mtx:          new(sync.Mutex),
		ready:        make(chan bool),
		allok:        true,
		zerostate: &zerostate{
			mtx:             new(sync.Mutex),
			delayedMessages: make(map[string]*types.Message),
		},
		onestate: &onestate{
			mtx:              new(sync.Mutex),
			delayedProposals: make(map[string]*types.Message),
		},
		twostate: &twostate{
			mtx:              new(sync.Mutex),
			seenProposal:     false,
			delayedDelivered: false,
		},
	}
}

func (l *LockedValue) Initialize(replicaStore *types.ReplicaStore, logger *log.Logger) (testing.TestCaseCtx, error) {

	l.allReplica = replicaStore
	faults := int((replicaStore.Size() - 1) / 3)
	l.BaseTestCase.Initialize(replicaStore, logger)
	l.partitioner = roundskip.NewStaticPartitioner(replicaStore, faults, logger)
	l.partitioner.NewPartition(0)

	close(l.ready)
	return l.BaseTestCase.Ctx, nil
}

type zerostate struct {
	mtx             *sync.Mutex
	delayedMessages map[string]*types.Message
}

func (l *LockedValue) handleRoundzero(msg *types.Message) (bool, []*types.Message) {
	// Ensure only one replica sees the consensus and then wait for roundskip before incrementing curRound
	tMsg, err := util.Unmarshal(msg.Msg)
	if err != nil {
		return true, []*types.Message{}
	}

	if tMsg.Type != util.Prevote {
		return true, []*types.Message{}
	}

	partition, ok := l.partitioner.GetPartition(0)
	if ok {
		honestDelayed, ok := partition.GetPart("honestDelayed")
		if ok && honestDelayed.Exists(msg.From) {
			l.zerostate.mtx.Lock()
			l.zerostate.delayedMessages[msg.ID] = msg
			l.zerostate.mtx.Unlock()
			return false, []*types.Message{}
		}
		faulty, ok := partition.GetPart("faulty")
		if ok && faulty.Exists(msg.From) {
			replica, ok := l.allReplica.GetReplica(msg.From)
			if !ok {
				return true, []*types.Message{}
			}
			newvote, err := util.ChangeVote(replica, tMsg)
			if err != nil {
				return true, []*types.Message{}
			}
			newvoteB, err := util.Marshal(newvote)
			if err != nil {
				return true, []*types.Message{}
			}
			newmsg := msg.Clone()
			newmsg.ID = ""
			newmsg.Msg = newvoteB
			return false, []*types.Message{newmsg}
		}
	}
	return true, []*types.Message{}
}

type onestate struct {
	mtx              *sync.Mutex
	delayedProposals map[string]*types.Message
}

func (l *LockedValue) handleRoundone(msg *types.Message) (bool, []*types.Message) {
	// Don't deliver proposal and wait for nil prevotes from everybody and eventually round skip
	tMsg, err := util.Unmarshal(msg.Msg)
	if err != nil {
		return true, []*types.Message{}
	}
	if tMsg.Type == util.Proposal {
		l.onestate.mtx.Lock()
		l.onestate.delayedProposals[msg.ID] = msg
		l.onestate.mtx.Unlock()
		return false, []*types.Message{}
	} else if tMsg.Type == util.Prevote {
		partition, ok := l.partitioner.GetPartition(0)
		if ok {
			faulty, ok := partition.GetPart("faulty")
			if ok && faulty.Exists(msg.From) {
				replica, ok := l.allReplica.GetReplica(msg.From)
				if !ok {
					return true, []*types.Message{}
				}
				newvote, err := util.ChangeVote(replica, tMsg)
				if err != nil {
					return true, []*types.Message{}
				}
				newvoteB, err := util.Marshal(newvote)
				if err != nil {
					return true, []*types.Message{}
				}
				newmsg := msg.Clone()
				newmsg.ID = ""
				newmsg.Msg = newvoteB
				return false, []*types.Message{newmsg}
			}
		}
	}
	return true, []*types.Message{}
}

type twostate struct {
	mtx              *sync.Mutex
	seenProposal     bool
	delayedDelivered bool
}

func (l *LockedValue) handleRoundtwo(msg *types.Message) (bool, []*types.Message) {
	// Deliver delayed prevotes from round 0 and then see what gets committed

	tMsg, err := util.Unmarshal(msg.Msg)
	if err != nil {
		return true, []*types.Message{}
	}
	if tMsg.Type == util.Proposal {
		_, r := util.ExtractHR(tMsg)
		if r == 2 {
			l.twostate.mtx.Lock()
			l.twostate.seenProposal = true
			l.twostate.mtx.Unlock()
		}
	} else if tMsg.Type == util.Prevote {
		partition, ok := l.partitioner.GetPartition(0)
		if ok {
			faulty, ok := partition.GetPart("faulty")
			if ok && faulty.Exists(msg.From) {
				replica, ok := l.allReplica.GetReplica(msg.From)
				if !ok {
					return true, []*types.Message{}
				}
				newvote, err := util.ChangeVote(replica, tMsg)
				if err != nil {
					return true, []*types.Message{}
				}
				newvoteB, err := util.Marshal(newvote)
				if err != nil {
					return true, []*types.Message{}
				}
				newmsg := msg.Clone()
				newmsg.ID = ""
				newmsg.Msg = newvoteB
				return false, []*types.Message{newmsg}
			}
		}
	}
	l.twostate.mtx.Lock()
	delayedDelivered := l.twostate.delayedDelivered
	seenProposal := l.twostate.seenProposal
	l.twostate.mtx.Unlock()
	if !seenProposal {
		return true, []*types.Message{}
	}

	if !delayedDelivered {
		l.Logger.Info("Delivering old prevotes")
		l.zerostate.mtx.Lock()
		extra := make([]*types.Message, len(l.zerostate.delayedMessages))
		i := 0
		for _, m := range l.zerostate.delayedMessages {
			extra[i] = m
			i++
		}
		for k := range l.zerostate.delayedMessages {
			delete(l.zerostate.delayedMessages, k)
		}
		l.zerostate.mtx.Unlock()
		l.twostate.mtx.Lock()
		l.twostate.delayedDelivered = true
		l.twostate.mtx.Unlock()
		return true, extra
	}
	return true, []*types.Message{}
}

func (l *LockedValue) HandleMessage(msg *types.Message) (bool, []*types.Message) {
	<-l.ready
	tMsg, err := util.Unmarshal(msg.Msg)
	if err != nil {
		return true, []*types.Message{}
	}
	h, r := util.ExtractHR(tMsg)

	if h != 1 {
		return true, []*types.Message{}
	}

	switch r {
	case 0:
		return l.handleRoundzero(msg)
	case 1:
		return l.handleRoundone(msg)
	case 2:
		return l.handleRoundtwo(msg)
	}
	return true, []*types.Message{}
}

func (l *LockedValue) Assert() error {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	if l.allok {
		return nil
	}
	return errors.New("testcase failed")
}
