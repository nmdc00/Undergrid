//go:build hp1_live && hp2_live

package network

import "testing"

func TestHP2LiveContinuousDiscoveryAdmissionPlanAndRecovery(t *testing.T) {
	hp2LiveOwnerSetup(t, hp2Continuous)
}
