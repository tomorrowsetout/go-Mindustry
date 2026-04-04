package main

import (
	"reflect"
	"testing"
	"time"
)

func TestNeededVotesUsesStrictMajority(t *testing.T) {
	cases := []struct {
		total int
		want  int
	}{
		{total: 0, want: 1},
		{total: 1, want: 1},
		{total: 2, want: 2},
		{total: 3, want: 2},
		{total: 4, want: 3},
	}

	for _, tc := range cases {
		if got := neededVotes(tc.total); got != tc.want {
			t.Fatalf("neededVotes(%d) = %d, want %d", tc.total, got, tc.want)
		}
	}
}

func TestMapVoteBeginVoteAutoAddsStarterVote(t *testing.T) {
	rt := &mapVoteRuntime{
		duration: time.Minute,
	}

	snapshot, result, err := rt.beginVote("maps/alpha.msav", "alpha", "starter", "Alice", 4)
	if err != nil {
		t.Fatalf("begin vote failed: %v", err)
	}
	if result.Decision != mapVoteDecisionPending {
		t.Fatalf("expected pending result after begin, got %v", result.Decision)
	}
	if snapshot.MapName != "alpha" || snapshot.StartedBy != "Alice" {
		t.Fatalf("unexpected vote snapshot: %+v", snapshot)
	}
	if snapshot.Yes != 1 || snapshot.No != 0 {
		t.Fatalf("expected starter auto-vote yes=1 no=0, got yes=%d no=%d", snapshot.Yes, snapshot.No)
	}
	if snapshot.Needed != 3 {
		t.Fatalf("expected vote need=3 for 4 players, got %d", snapshot.Needed)
	}
	if rt.active == nil || rt.active.Votes["starter"] != 1 {
		t.Fatalf("expected active vote session with starter yes vote, got %+v", rt.active)
	}
	if rt.active.Timer == nil {
		t.Fatal("expected active vote timer to be created")
	}
	rt.active.Timer.Stop()
}

func TestMapVoteCastVotePassesOnMajority(t *testing.T) {
	rt := &mapVoteRuntime{
		duration: time.Minute,
	}

	if _, _, err := rt.beginVote("maps/alpha.msav", "alpha", "starter", "Alice", 3); err != nil {
		t.Fatalf("begin vote failed: %v", err)
	}
	resultHolder := rt.active
	snapshot, result, err := rt.castVote("second", 1, 3)
	if err != nil {
		t.Fatalf("cast vote failed: %v", err)
	}
	if snapshot != (mapVoteSnapshot{}) {
		t.Fatalf("expected empty snapshot after passing vote, got %+v", snapshot)
	}
	if result.Decision != mapVoteDecisionPassed {
		t.Fatalf("expected passed vote result, got %v", result.Decision)
	}
	if result.Yes != 2 || result.No != 0 || result.Needed != 2 {
		t.Fatalf("unexpected passed vote result: %+v", result)
	}
	if rt.active != nil {
		t.Fatalf("expected active vote to clear after pass, got %+v", rt.active)
	}
	if resultHolder != nil && resultHolder.Timer != nil {
		resultHolder.Timer.Stop()
	}
}

func TestMapVoteCastVoteRejectsWithoutMajorityWhenAllPlayersVoted(t *testing.T) {
	rt := &mapVoteRuntime{
		duration: time.Minute,
	}

	if _, _, err := rt.beginVote("maps/beta.msav", "beta", "starter", "Alice", 2); err != nil {
		t.Fatalf("begin vote failed: %v", err)
	}
	resultHolder := rt.active
	_, result, err := rt.castVote("second", -1, 2)
	if err != nil {
		t.Fatalf("cast vote failed: %v", err)
	}
	if result.Decision != mapVoteDecisionRejected {
		t.Fatalf("expected rejected vote result, got %v", result.Decision)
	}
	if result.Yes != 1 || result.No != 1 || result.Needed != 2 {
		t.Fatalf("unexpected rejected vote result: %+v", result)
	}
	if rt.active != nil {
		t.Fatalf("expected active vote to clear after rejection, got %+v", rt.active)
	}
	if resultHolder != nil && resultHolder.Timer != nil {
		resultHolder.Timer.Stop()
	}
}

func TestMapVoteCastNeutralKeepsVotePending(t *testing.T) {
	rt := &mapVoteRuntime{
		duration: time.Minute,
	}

	snapshot, _, err := rt.beginVote("maps/beta.msav", "beta", "starter", "Alice", 3)
	if err != nil {
		t.Fatalf("begin vote failed: %v", err)
	}
	if snapshot.Yes != 1 || snapshot.Neutral != 0 {
		t.Fatalf("unexpected initial snapshot: %+v", snapshot)
	}

	snapshot, result, err := rt.castVote("second", 0, 3)
	if err != nil {
		t.Fatalf("cast neutral vote failed: %v", err)
	}
	if result.Decision != mapVoteDecisionPending {
		t.Fatalf("expected neutral vote to keep vote pending, got %v", result.Decision)
	}
	if snapshot.Neutral != 1 || snapshot.Yes != 1 || snapshot.No != 0 {
		t.Fatalf("unexpected snapshot after neutral vote: %+v", snapshot)
	}
	if rt.active == nil || rt.active.Votes["second"] != 0 {
		t.Fatalf("expected neutral vote to be recorded, got %+v", rt.active)
	}
	if rt.active.Timer != nil {
		rt.active.Timer.Stop()
	}
}

func TestMapVoteListPagePaginatesMaps(t *testing.T) {
	rt := &mapVoteRuntime{
		listMaps: func() ([]string, error) {
			return []string{"alpha", "beta", "gamma", "delta", "omega"}, nil
		},
	}

	first, page, total, err := rt.listPage(0)
	if err != nil {
		t.Fatalf("list page 0 failed: %v", err)
	}
	if page != 0 || total != 2 {
		t.Fatalf("expected first page metadata (0,2), got (%d,%d)", page, total)
	}
	if !reflect.DeepEqual(first, []string{"alpha", "beta", "gamma", "delta"}) {
		t.Fatalf("unexpected first page maps: %#v", first)
	}

	second, page, total, err := rt.listPage(1)
	if err != nil {
		t.Fatalf("list page 1 failed: %v", err)
	}
	if page != 1 || total != 2 {
		t.Fatalf("expected second page metadata (1,2), got (%d,%d)", page, total)
	}
	if !reflect.DeepEqual(second, []string{"omega"}) {
		t.Fatalf("unexpected second page maps: %#v", second)
	}
}

func TestMapVoteSelectionOptionsIncludeHomeLinkRow(t *testing.T) {
	got := mapVoteSelectionOptions([]string{"alpha", "beta", "gamma"}, 0, 2, true)
	want := [][]string{
		{"alpha", "beta"},
		{"gamma"},
		{"下一页"},
		{"打开链接", "关闭"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected map vote options with link: %#v", got)
	}
}

func TestMapVoteExpireNotifiesConfiguredCallback(t *testing.T) {
	results := make(chan mapVoteResult, 1)
	rt := &mapVoteRuntime{
		duration: time.Minute,
		notifyResult: func(result mapVoteResult) {
			results <- result
		},
	}

	if _, _, err := rt.beginVote("maps/alpha.msav", "alpha", "starter", "Alice", 3); err != nil {
		t.Fatalf("begin vote failed: %v", err)
	}
	active := rt.active
	if active == nil {
		t.Fatal("expected active vote before expiration")
	}
	if active.Timer != nil {
		active.Timer.Stop()
	}

	rt.expireVote(active.Token)

	select {
	case result := <-results:
		if result.Decision != mapVoteDecisionExpired {
			t.Fatalf("expected expired vote result, got %v", result.Decision)
		}
		if result.MapName != "alpha" || result.Yes != 1 || result.No != 0 {
			t.Fatalf("unexpected expired vote result: %+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("expected expiration callback to be invoked")
	}
}
