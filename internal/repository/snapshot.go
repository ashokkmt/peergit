package repository

import (
	"context"
	"errors"
	"io"
	"regexp"

	"peergit/internal/platform/storage"
)

type Snapshot struct {
	Provider                    string          `json:"provider"`
	ExternalRepositoryID        int64           `json:"external_repository_id"`
	Commit                      string          `json:"canonical_commit"`
	Object                      storage.Receipt `json:"object"`
	State                       string          `json:"state"`
	LFSAndSubmoduleCompleteness string          `json:"lfs_and_submodule_completeness"`
}

func Capture(ctx context.Context, store *storage.Store, repositoryID int64, sha string, archive io.Reader, limit int64) (Snapshot, error) {
	if repositoryID <= 0 || !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(sha) {
		return Snapshot{}, errors.New("snapshot needs immutable repository ID and exact commit SHA")
	}
	object, err := store.Capture(ctx, archive, "proofs/capture", limit)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{"github", repositoryID, "git:sha1:" + sha, object, "verified", "unverified; LFS objects and submodule contents are not independently captured"}, nil
}
