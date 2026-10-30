package api

import (
	"errors"
	"testing"
)

func TestQ11RuntimeHealthUsesClosedPiReasonSet(t *testing.T) {
	for _, reason := range []string{
		"observer_version_timeout", "observer_models_timeout",
		"observer_version_process", "observer_models_process",
		"observer_version_output_limit", "observer_models_output_limit",
		"observer_version_output", "observer_models_output",
		"observer_version_stderr", "observer_models_stderr", "observer_models_duplicate",
	} {
		t.Run(reason, func(t *testing.T) {
			service := &LocalProductReadService{runtimeHealth: &controlledRuntimeObservationHealthSource{reason: reason, partial: true}}
			got, err := service.applyRuntimeObservationHealth(LocalProductSnapshot{ViewVersion: "unchanged"})
			if err != nil || !got.Partial || got.Reason != reason || got.ViewVersion != "unchanged" {
				t.Fatalf("health = %#v, %v", got, err)
			}
		})
	}
	for _, tc := range []struct {
		reason  string
		partial bool
	}{
		{"observer_unknown", true}, {"observer_version_duplicate", true},
		{"observer_version_output secret", true}, {"", true},
		{"observer_version_output", false},
	} {
		service := &LocalProductReadService{runtimeHealth: &controlledRuntimeObservationHealthSource{reason: tc.reason, partial: tc.partial}}
		if _, err := service.applyRuntimeObservationHealth(LocalProductSnapshot{}); !errors.Is(err, ErrLocalProductStateUnavailable) {
			t.Fatalf("invalid health accepted: %#v, %v", tc, err)
		}
	}
}
