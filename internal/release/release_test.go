package release

import (
	"testing"
	"time"
)

func mustTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func TestSelectPrevious(t *testing.T) {
	releases := []Release{
		{ID: "20260618-120000-a111", CreatedAt: mustTime("2026-06-18T12:00:00Z")},
		{ID: "20260618-130000-b222", CreatedAt: mustTime("2026-06-18T13:00:00Z")},
		{ID: "20260618-140000-c333", CreatedAt: mustTime("2026-06-18T14:00:00Z")},
	}
	previous, err := SelectPrevious(releases, "20260618-140000-c333")
	if err != nil {
		t.Fatalf("SelectPrevious returned error: %v", err)
	}
	if previous.ID != "20260618-130000-b222" {
		t.Fatalf("previous.ID = %s", previous.ID)
	}
}

func TestSelectPreviousFailsForFirstRelease(t *testing.T) {
	releases := []Release{
		{ID: "20260618-120000-a111", CreatedAt: mustTime("2026-06-18T12:00:00Z")},
	}
	if _, err := SelectPrevious(releases, "20260618-120000-a111"); err == nil {
		t.Fatal("expected error")
	}
}

func TestPruneCandidatesKeepCurrentAndNewest(t *testing.T) {
	releases := []Release{
		{ID: "20260618-110000-a111", CreatedAt: mustTime("2026-06-18T11:00:00Z")},
		{ID: "20260618-120000-b222", CreatedAt: mustTime("2026-06-18T12:00:00Z")},
		{ID: "20260618-130000-c333", CreatedAt: mustTime("2026-06-18T13:00:00Z")},
		{ID: "20260618-140000-d444", CreatedAt: mustTime("2026-06-18T14:00:00Z")},
	}
	candidates, err := PruneCandidates(releases, "20260618-120000-b222", 3)
	if err != nil {
		t.Fatalf("PruneCandidates returned error: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ID != "20260618-110000-a111" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
}

func TestPruneCandidatesRequiresCurrentRelease(t *testing.T) {
	releases := []Release{
		{ID: "20260618-110000-a111", CreatedAt: mustTime("2026-06-18T11:00:00Z")},
	}
	if _, err := PruneCandidates(releases, "20260618-120000-b222", 3); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseList(t *testing.T) {
	releases, err := ParseList("20260618-120000-a111\t2026-06-18T12:00:00Z\n")
	if err != nil {
		t.Fatalf("ParseList returned error: %v", err)
	}
	if len(releases) != 1 || releases[0].ID != "20260618-120000-a111" {
		t.Fatalf("unexpected releases: %+v", releases)
	}
}
