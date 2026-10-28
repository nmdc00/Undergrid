//go:build hp1_live && hp2_live

package network

import "testing"

func TestHP2LiveOwnerClarifiesCurrentPublicRequirements(t *testing.T) {
	hp2LiveOwnerSetup(t, hp2Clarifications)
}
