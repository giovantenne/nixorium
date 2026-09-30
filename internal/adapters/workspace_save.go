package adapters

import (
	"bytes"
	"context"
	"errors"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

func (local Local) RecordWorkspaceCandidate(ctx context.Context, repository, expectedRevision string, candidate domain.WorkspaceProfile) (string, error) {
	data, err := domain.MarshalWorkspaceProfile(candidate)
	if err != nil {
		return "", err
	}
	root, err := openWorkspaceRoot(repository, unix.LOCK_EX)
	if err != nil {
		return "", err
	}
	defer root.Close()
	if _, err := workspaceRootIdentity(root); err != nil {
		return "", err
	}
	current, info, err := readWorkspaceFile(root, domain.WorkspaceFileName, domain.WorkspaceMaxBytes)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(data, current) || info.Nlink != 1 || info.Mode&0777 != 0600 {
		return "", domain.ErrWorkspaceConflict
	}
	revision, err := local.GitRevision(ctx, repository)
	if err != nil || revision != expectedRevision {
		return "", domain.ErrWorkspaceConflict
	}
	staged, truncated, err := runBoundedGit(ctx, repository, 4096, "diff", "--cached", "--name-only", "-z", "--no-ext-diff", "--no-textconv", "HEAD", "--", domain.WorkspaceFileName)
	if err != nil || truncated || staged != "" {
		return "", domain.ErrWorkspaceConflict
	}
	paths := []string{domain.WorkspaceFileName}
	proposal, err := local.GitCommitProposal(ctx, repository, paths)
	if err != nil {
		return "", err
	}
	// Bind authorization to the immutable proposed blob, not a separate file
	// read that could race with staging. CommitGitPaths rechecks this exact tree.
	blob, truncated, err := runBoundedGit(ctx, repository, domain.WorkspaceMaxBytes, "cat-file", "blob", proposal.TreeID+":"+domain.WorkspaceFileName)
	if err != nil || truncated || !bytes.Equal([]byte(blob), data) {
		return "", errors.New("workspace commit candidate differs from the reviewed save")
	}
	if _, err := workspaceRootIdentity(root); err != nil {
		return "", err
	}
	return local.CommitGitPaths(ctx, repository, paths, "chore: update student workspace", expectedRevision, proposal.TreeID)
}
