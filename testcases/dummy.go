package testcases

import (
	"time"

	"github.com/ds-test-framework/scheduler/testing"
	"github.com/ds-test-framework/scheduler/types"
)

type DummyTestCase struct {
	testing.BaseTestCase
}

func NewDummtTestCase() *DummyTestCase {
	d := &DummyTestCase{}
	d.BaseTestCase = *testing.NewBaseTestCase("Dummy", 10*time.Second)
	return d
}

func (d *DummyTestCase) HandleMessage(_ *types.Message) (bool, []*types.Message) {
	return true, []*types.Message{}
}

func (d *DummyTestCase) Assert() error {
	return nil
}
