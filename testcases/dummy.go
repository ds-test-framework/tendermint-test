package testcases

import (
	"time"

	"github.com/ds-test-framework/scheduler/log"
	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

func getAction(testcase *testlib.TestCase) testlib.StateAction {
	return func(c *testlib.Context) []*types.Message {
		event := c.CurEvent
		switch event.Type.(type) {
		case *types.MessageSendEventType:
			eventType := event.Type.(*types.MessageSendEventType)
			testcase.Logger.With(log.LogParams{"message_id": eventType.MessageID}).Info("Received message")
			message, ok := c.MessagePool.Get(eventType.MessageID)
			if ok {
				return []*types.Message{message}
			}
		}
		return []*types.Message{}
	}
}

func getCond(testcase *testlib.TestCase) testlib.Condition {
	return func(c *testlib.Context) bool {
		e := c.CurEvent
		switch e.Type.(type) {
		case *types.MessageSendEventType:
			eventType := e.Type.(*types.MessageSendEventType)
			testcase.Logger.With(log.LogParams{"message_id": eventType.MessageID}).Info("Received message")
			messageRaw, ok := c.MessagePool.Get(eventType.MessageID)
			if ok {
				message, err := util.Unmarshal(messageRaw.Data)
				if err != nil {
					return false
				}
				if message.Type == util.Precommit {
					return true
				}
			}

		}
		return false
	}
}

func DummyTestCase() *testlib.TestCase {
	testcase := testlib.NewTestCase("Dummy", 30*time.Second)
	testcase.Start().Action = getAction(testcase)

	builder := testcase.Builder()
	builder.On(getCond(testcase), testcase.Success().Label)

	return testcase
}

// type dummyCond struct{}

// func (*dummyCond) Check(_ *testing.EventWrapper, _ *testing.VarSet) bool {
// 	return true
// }

// func NewDummtTestCase() *testing.TestCase {
// 	t := testing.NewTestCase("Dummy", 5*time.Second)

// 	t.StartState().Upon(&dummyCond{}, t.SuccessState())
// 	return t
// }
