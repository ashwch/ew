package ui

import "testing"

func TestBackendCandidatesAuto(t *testing.T) {
	got := backendCandidates("auto")
	want := []string{BackendBubbleTea, BackendHuh, BackendTView}
	assertBackendOrder(t, got, want)
}

func TestBackendCandidatesBubbleTeaFallsBack(t *testing.T) {
	got := backendCandidates("bubbletea")
	want := []string{BackendBubbleTea, BackendHuh, BackendTView}
	assertBackendOrder(t, got, want)
}

func TestBackendCandidatesHuhFallsBack(t *testing.T) {
	got := backendCandidates("huh")
	want := []string{BackendHuh, BackendBubbleTea, BackendTView}
	assertBackendOrder(t, got, want)
}

func TestBackendCandidatesTViewFallsBack(t *testing.T) {
	got := backendCandidates("tview")
	want := []string{BackendTView, BackendBubbleTea, BackendHuh}
	assertBackendOrder(t, got, want)
}

func TestBackendCandidatesPlain(t *testing.T) {
	got := backendCandidates("plain")
	want := []string{BackendPlain}
	assertBackendOrder(t, got, want)
}

func assertBackendOrder(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("unexpected candidate length: got=%d want=%d", len(got), len(want))
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("candidate[%d] mismatch: got=%q want=%q", idx, got[idx], want[idx])
		}
	}
}
