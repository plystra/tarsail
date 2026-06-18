package release

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Release struct {
	ID        string
	CreatedAt time.Time
}

var idRe = regexp.MustCompile(`^\d{8}-\d{6}-[a-f0-9]{4}$`)

func NewID(now time.Time) (string, error) {
	random := make([]byte, 2)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("[release:id] Could not generate release ID: %w", err)
	}
	return now.UTC().Format("20060102-150405") + "-" + hex.EncodeToString(random), nil
}

func ValidateID(id string) error {
	if !idRe.MatchString(id) {
		return fmt.Errorf("[release:id] Invalid release ID: %s", id)
	}
	return nil
}

func ParseList(output string) ([]Release, error) {
	var releases []Release
	for lineNo, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 2 {
			return nil, fmt.Errorf("[release:list] Invalid release list line %d: %q", lineNo+1, line)
		}
		if err := ValidateID(parts[0]); err != nil {
			return nil, err
		}
		createdAt, err := time.Parse(time.RFC3339, parts[1])
		if err != nil {
			return nil, fmt.Errorf("[release:list] Invalid created_at for release %s: %w", parts[0], err)
		}
		releases = append(releases, Release{ID: parts[0], CreatedAt: createdAt})
	}
	Sort(releases)
	return releases, nil
}

func Sort(releases []Release) {
	sort.Slice(releases, func(i, j int) bool {
		if releases[i].CreatedAt.Equal(releases[j].CreatedAt) {
			return releases[i].ID < releases[j].ID
		}
		return releases[i].CreatedAt.Before(releases[j].CreatedAt)
	})
}

func SelectPrevious(releases []Release, currentID string) (Release, error) {
	if err := ValidateID(currentID); err != nil {
		return Release{}, err
	}
	Sort(releases)
	for i, rel := range releases {
		if rel.ID != currentID {
			continue
		}
		if i == 0 {
			return Release{}, fmt.Errorf("[rollback:select] No previous release exists.\n\nWhy it matters:\n  Rollback can only restore a previously deployed release.")
		}
		return releases[i-1], nil
	}
	return Release{}, fmt.Errorf("[rollback:select] Current release %s was not found in the release list.", currentID)
}

func PruneCandidates(releases []Release, currentID string, keep int) ([]Release, error) {
	if keep < 1 {
		return nil, fmt.Errorf("[prune:select] keep must be at least 1.")
	}
	if err := ValidateID(currentID); err != nil {
		return nil, err
	}
	Sort(releases)
	currentFound := false
	for _, rel := range releases {
		if rel.ID == currentID {
			currentFound = true
			break
		}
	}
	if !currentFound {
		return nil, fmt.Errorf("[prune:select] Current release %s was not found in the release list.", currentID)
	}
	keepIDs := map[string]struct{}{currentID: {}}
	for i := len(releases) - 1; i >= 0 && len(keepIDs) < keep; i-- {
		keepIDs[releases[i].ID] = struct{}{}
	}

	var candidates []Release
	for _, rel := range releases {
		if _, keepRelease := keepIDs[rel.ID]; keepRelease {
			continue
		}
		candidates = append(candidates, rel)
	}
	return candidates, nil
}
