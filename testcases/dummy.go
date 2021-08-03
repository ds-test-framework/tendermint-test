package testcases

import (
	"time"

	"github.com/ds-test-framework/scheduler/testing"
)

type dummyCond struct{}

func (*dummyCond) Check(_ *testing.EventWrapper, _ *testing.VarSet) bool {
	return true
}

func NewDummtTestCase() *testing.TestCase {
	t := testing.NewTestCase("Dummy", 5*time.Second)

	t.StartState().Upon(&dummyCond{}, t.SuccessState())
	return t
}
