package testcases

import (
	"time"

	"github.com/ds-test-framework/scheduler/testlib"
	handlers "github.com/ds-test-framework/scheduler/testlib/handlers"
	smlib "github.com/ds-test-framework/scheduler/testlib/statemachine"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

func handler(e *types.Event, c *testlib.Context) ([]*types.Message, bool) {
	if !e.IsMessageSend() {
		return []*types.Message{}, false
	}
	messageID, _ := e.MessageID()
	message, ok := c.MessagePool.Get(messageID)
	if ok {
		return []*types.Message{message}, true
	}
	return []*types.Message{}, true
}

func cond(e *types.Event, c *testlib.Context) bool {
	if !e.IsMessageSend() {
		return false
	}

	message, ok := util.GetMessageFromEvent(e, c)
	if !ok {
		return false
	}
	return message.Type == util.Precommit
}

func DummyTestCase() *testlib.TestCase {
	testcase := testlib.NewTestCase("Dummy", 20*time.Second, handlers.NewGenericHandler(handler))
	testcase.AssertFn(func(c *testlib.Context) bool {
		return true
	})
	return testcase
}

func DummyTestCaseStateMachine() *testlib.TestCase {
	sm := smlib.NewStateMachine()
	sm.Builder().On(cond, smlib.SuccessStateLabel)

	h := handlers.NewHandlerCascade()
	h.AddHandler(handler)
	h.AddHandler(smlib.NewAsyncStateMachineHandler(sm))

	testcase := testlib.NewTestCase("DummySM", 30*time.Second, h)
	return testcase
}
