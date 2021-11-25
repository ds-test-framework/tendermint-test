package rskip

import (
	"sync"
	"time"

	"github.com/ds-test-framework/scheduler/testlib"
	smlib "github.com/ds-test-framework/scheduler/testlib/statemachine"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

func commit_or_round1(c *smlib.Context) bool {
	if c.CurEvent.IsMessageSend() {
		id, _ := c.CurEvent.MessageID()
		message, _ := c.MessagePool.Get(id)

		t, _ := util.Unmarshal(message.Data)
		_, r := util.ExtractHR(t)

		if r > 0 {
			return true
		}
	}

	eType, ok := c.CurEvent.Type.(*types.GenericEventType)
	if ok && eType.T == "Committing block" {
		return true
	}

	return false
}

type counter struct {
	counts map[types.ReplicaID]int
	lock   *sync.Mutex
}

func newCounter() *counter {
	return &counter{
		counts: make(map[types.ReplicaID]int),
		lock:   new(sync.Mutex),
	}
}

func (ctr *counter) Count(r types.ReplicaID) int {
	ctr.lock.Lock()
	defer ctr.lock.Unlock()
	c, ok := ctr.counts[r]
	if !ok {
		return 0
	}
	return c
}

func (ctr *counter) Incr(r types.ReplicaID) {
	ctr.lock.Lock()
	defer ctr.lock.Unlock()

	_, ok := ctr.counts[r]
	if !ok {
		ctr.counts[r] = 1
	} else {
		ctr.counts[r] = ctr.counts[r] + 1
	}
}

func filter_less_than_n_minus_f(c *smlib.Context) ([]*types.Message, bool) {
	if !c.CurEvent.IsMessageSend() {
		return []*types.Message{}, true
	}
	messageID, _ := c.CurEvent.MessageID()
	message, ok := c.MessagePool.Get(messageID)
	if !ok {
		return []*types.Message{}, false
	}

	tMsg, err := util.Unmarshal(message.Data)
	if err != nil {
		return []*types.Message{}, false
	}
	if tMsg.Type != util.Prevote {
		return []*types.Message{message}, false
	}
	if !c.Vars.Exists("msgCounter") {
		ctr := newCounter()
		c.Vars.Set("msgCounter", ctr)
	}
	ctrI, _ := c.Vars.Get("msgCounter")
	ctr := ctrI.(*counter)

	n := c.Replicas.Cap()
	f := n / 3
	count := ctr.Count(message.To)
	if count < f+1 {
		delayedM := getDelayedMStore(c.Context)
		delayedM.Add(message)
		ctr.Incr(message.To)
		return []*types.Message{}, true
	}
	return []*types.Message{message}, true
}

func BlockingTestcase() *testlib.TestCase {

	sm := smlib.NewStateMachine()
	sm.Builder().
		On(commit_or_round1, smlib.FailStateLabel)

	handler := smlib.NewAsyncStateMachineHandler(sm)
	handler.AddEventHandler(filter_less_than_n_minus_f)
	handler.AddEventHandler(deliverDelayedFilter)

	testcase := testlib.NewTestCase("BlockingTestCase", 30*time.Second, handler)
	testcase.SetupFunc(setupFunc)
	testcase.AssertFn(func(c *testlib.Context) bool {
		cmr1, ok := c.Vars.GetBool("cmr1")
		return !ok || !cmr1
	})

	return testcase
}
