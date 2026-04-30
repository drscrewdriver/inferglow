package sandbox

import (
	"testing"
)

func TestDockerProviderNameKind(t *testing.T) {
	_, err := NewDockerProvider()
	if err != nil {
		t.Skip("Docker not available, skipping Name/Kind check")
	}
}

func TestDockerProviderInspectAvailability(t *testing.T) {
	_, err := NewDockerProvider()
	if err != nil {
		t.Skip("Docker not available, skipping InspectAvailability check")
	}
}

func TestDockerProviderInspectAvailabilityNotAvailable(t *testing.T) {
	_, err := NewDockerProvider()
	if err != nil {
		// Docker not available — can't test InspectAvailability
		t.Skip("Docker not available")
	}
}
