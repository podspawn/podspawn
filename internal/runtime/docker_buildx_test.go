package runtime

import "testing"

func TestHasBuildxRespectsDisableEnv(t *testing.T) {
	t.Setenv("PODSPAWN_DISABLE_BUILDX", "1")
	if hasBuildx() {
		t.Error("hasBuildx returned true when PODSPAWN_DISABLE_BUILDX is set")
	}
}
