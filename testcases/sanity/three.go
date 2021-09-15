package sanity

import (
	"time"

	"github.com/ds-test-framework/scheduler/testlib"
)

func ThreeTestCase() *testlib.TestCase {
	testcase := testlib.NewTestCase("LockedValueCheck", 30*time.Second)
	return testcase
}
