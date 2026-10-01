package consumer

import "testing"

func TestEnsureCorrelationIDPreservesRetries(t *testing.T) {
	if got := ensureCorrelationID("corr-retry", "event-1"); got != "corr-retry" {
		t.Fatalf("got %q", got)
	}
}

func TestEnsureCorrelationIDGeneratesLegacyFallback(t *testing.T) {
	first := ensureCorrelationID("", "event-legacy")
	second := ensureCorrelationID("", "event-legacy")
	if first == "" || first != second {
		t.Fatalf("expected stable fallback IDs: %q %q", first, second)
	}
	first = ensureCorrelationID("", "")
	second = ensureCorrelationID("", "")
	if first == "" || second == "" || first == second {
		t.Fatalf("expected distinct fallback IDs: %q %q", first, second)
	}
}
