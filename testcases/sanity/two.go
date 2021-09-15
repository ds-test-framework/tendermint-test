package sanity

import (
	"time"

	"github.com/ds-test-framework/scheduler/testlib"
)

// States:
// 	1. Ensure replicas skip round by not delivering enough precommits
//		1.1 One replica prevotes and precommits nil
// 	2. In the next round change the proposal block value
// 	3. Replicas should prevote and precommit nil and hence round skip
func TwoTestCase() *testlib.TestCase {
	testcase := testlib.NewTestCase("WrongProposal", 30*time.Second)
	return testcase
}
