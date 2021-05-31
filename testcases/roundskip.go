package testcases

import (
	"errors"
	"sync"
	"time"

	"github.com/ds-test-framework/scheduler/testing"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

type RoundSkipTest struct {
	*testing.BaseTestCase

	fPeers          map[types.ReplicaID]int
	fPeersCount     int
	mtx             *sync.Mutex
	faults          int
	roundSkips      map[types.ReplicaID]bool
	roundSkipsCount int
	roundSkipped    bool
	height          int
}

func NewRoundSkipTest() *RoundSkipTest {
	return &RoundSkipTest{
		BaseTestCase:    testing.NewBaseTestCase("RoundSkip", 10*time.Second),
		fPeers:          make(map[types.ReplicaID]int),
		fPeersCount:     0,
		mtx:             new(sync.Mutex),
		roundSkips:      make(map[types.ReplicaID]bool),
		faults:          2,
		roundSkipsCount: 0,
		roundSkipped:    false,
		height:          -1,
	}
}

func (r *RoundSkipTest) Initialize(replicas *types.ReplicaStore) (testing.TestCaseCtx, error) {
	r.faults = int((replicas.Count() - 1) / 3)
	return r.BaseTestCase.Ctx, nil
}

func (r *RoundSkipTest) Assert() error {
	if r.roundSkipped {
		return nil
	}
	return errors.New("could not skip rounds")
}

func (r *RoundSkipTest) HandleMessage(m *types.Message) (bool, []*types.Message) {

	msg, err := util.Unmarshal(m.Msg)
	if err != nil {
		return true, []*types.Message{}
	}
	// 1. If round skip done then don't do anything
	r.mtx.Lock()
	skipped := r.roundSkipped
	r.mtx.Unlock()
	if skipped {
		return true, []*types.Message{}
	}
	// 2. Check if peer is something we should block for
	block := r.checkPeer(msg)
	if !block {
		return true, []*types.Message{}
	}
	// 3. Record data about peer. (Round number from any vote message or NewRoundStep)
	block = r.recordNCheck(msg)
	if !block {
		return true, []*types.Message{}
	}
	// 4. BlockPart messages should be blocked and the rest allowed
	return msg.Type != "BlockPart", []*types.Message{}
}

func (e *RoundSkipTest) checkPeer(msg *util.TendermintMessage) bool {
	e.mtx.Lock()
	defer e.mtx.Unlock()
	_, ok := e.fPeers[msg.To]
	if ok {
		return true
	}
	if e.fPeersCount == e.faults+1 {
		return false
	}
	e.fPeers[msg.To] = 0
	e.fPeersCount = e.fPeersCount + 1
	e.roundSkips[msg.To] = false
	return true
}

func (e *RoundSkipTest) recordNCheck(msg *util.TendermintMessage) bool {

	switch msg.Type {
	case "NewRoundStep":
		hrs := msg.Msg.GetNewRoundStep()
		return e.checkHeightRound(msg.From, int(hrs.Height), int(hrs.Round))
	}

	return true
}

func (e *RoundSkipTest) checkHeightRound(peer types.ReplicaID, height, round int) bool {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	// Uncomment to induce round skip only on the first instance
	// if e.height == -1 {
	// 	e.height = height
	// }
	if height < e.height {
		return false
	} else if height > e.height {
		// Set roundSkipped to true and comment rest if you want to induce round skip only for first instance
		e.height = height
		e.roundSkipped = false
		e.roundSkipsCount = 0
		for peer := range e.fPeers {
			e.fPeers[peer] = 0
			e.roundSkips[peer] = false
		}
	} else if height == e.height {
		curRound, ok := e.fPeers[peer]
		if !ok {
			return false
		}
		if curRound < round {
			e.fPeers[peer] = round
			if !e.roundSkips[peer] {
				e.roundSkips[peer] = true
				e.roundSkipsCount = e.roundSkipsCount + 1
				if e.roundSkipsCount == e.faults+1 {
					e.roundSkipped = true
					e.BaseTestCase.Ctx.SetDone()
				}
			}
		}
	}
	return true
}
