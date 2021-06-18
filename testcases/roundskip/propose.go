package roundskip

import (
	"errors"
	"sync"
	"time"

	"github.com/ds-test-framework/scheduler/log"
	"github.com/ds-test-framework/scheduler/testing"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

type RoundSkipBlockPart struct {
	*testing.BaseTestCase

	fPeers       map[types.ReplicaID]int
	fPeersCount  int
	mtx          *sync.Mutex
	faults       int
	curRound     int
	flag         bool
	height       int
	roundsToSkip int
}

func NewRoundSkipBlockPart(height, roundsToSkip int) *RoundSkipBlockPart {
	return &RoundSkipBlockPart{
		BaseTestCase: testing.NewBaseTestCase("RoundSkipBlockPart", 30*time.Second),
		fPeers:       make(map[types.ReplicaID]int),
		fPeersCount:  0,
		mtx:          new(sync.Mutex),
		roundsToSkip: roundsToSkip,
		curRound:     0,
		faults:       2,
		height:       height,
		flag:         false,
	}
}

func (r *RoundSkipBlockPart) Initialize(replicas *types.ReplicaStore, logger *log.Logger) (testing.TestCaseCtx, error) {
	r.BaseTestCase.Initialize(replicas, logger)
	r.faults = int((replicas.Size() - 1) / 3)
	return r.BaseTestCase.Ctx, nil
}

func (r *RoundSkipBlockPart) Assert() error {
	if r.flag {
		return nil
	}
	return errors.New("could not skip rounds")
}

func (r *RoundSkipBlockPart) HandleMessage(m *types.Message) (bool, []*types.Message) {

	msg, err := util.Unmarshal(m.Msg)
	if err != nil {
		return true, []*types.Message{}
	}
	// 1. If round skip done then don't do anything
	r.mtx.Lock()
	curHeight := r.height
	skipped := r.flag
	r.mtx.Unlock()
	if msg.Type == "None" || skipped {
		return true, []*types.Message{}
	}

	height, round := util.ExtractHR(msg)
	if height == -1 || round == -1 {
		return true, []*types.Message{}
	}
	if height != curHeight {
		return true, []*types.Message{}
	}

	r.record(msg.From, round)

	block := r.checkPeer(msg.To)
	if !block {
		return true, []*types.Message{}
	}

	// 4. BlockPart messages should be blocked and the rest allowed
	return msg.Type != "BlockPart", []*types.Message{}
}

func (e *RoundSkipBlockPart) record(peer types.ReplicaID, round int) {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	if e.fPeersCount != e.faults+1 {
		_, ok := e.fPeers[peer]
		if !ok {
			e.fPeers[peer] = round
		}
	}

	curPeerRound, ok := e.fPeers[peer]
	if !ok {
		return
	}
	if round > curPeerRound {
		e.fPeers[peer] = round
	}

	count := 0
	for _, round := range e.fPeers {
		if round > e.curRound {
			count = count + 1
		}
	}
	if count > e.faults {
		e.curRound = e.curRound + 1
		if e.curRound == e.roundsToSkip {
			e.flag = true
		}
	}

}

func (e *RoundSkipBlockPart) checkPeer(peer types.ReplicaID) bool {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	_, ok := e.fPeers[peer]
	return ok
}
