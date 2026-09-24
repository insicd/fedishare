package activitystreams

import (
	"time"

	"github.com/fedishare/fedishare/internal/files"
)

// AcceptFollow builds an Accept of a remote Follow.
func AcceptFollow(paths Paths, acceptID, followID, followerActor string) map[string]any {
	return map[string]any{
		"@context":  Context,
		"id":        acceptID,
		"type":      "Accept",
		"actor":     paths.Actor(),
		"published": time.Now().UTC().Format(time.RFC3339),
		"to":        []string{followerActor},
		"object": map[string]any{
			"id":     followID,
			"type":   "Follow",
			"actor":  followerActor,
			"object": paths.Actor(),
		},
	}
}

// FileUpdate is the same object as FileCreate with type Update and a
// distinct activity id so deliveries are not treated as duplicates.
func FileUpdate(paths Paths, rec files.Record, activityID string) map[string]any {
	act := FileCreate(paths, rec)
	act["id"] = activityID
	act["type"] = "Update"
	return act
}

// FileDelete announces that a shared file is no longer available.
func FileDelete(paths Paths, rec files.Record, activityID string) map[string]any {
	return map[string]any{
		"@context":  Context,
		"id":        activityID,
		"type":      "Delete",
		"actor":     paths.Actor(),
		"published": time.Now().UTC().Format(time.RFC3339),
		"to":        []string{PublicAudience},
		"cc":        []string{paths.Followers()},
		"object": map[string]any{
			"id":         paths.Note(rec.ID),
			"type":       "Tombstone",
			"formerType": "Note",
		},
	}
}
