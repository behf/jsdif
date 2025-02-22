package main

import (
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type CommitInfo struct {
	Hash string    `json:"hash"`
	Date time.Time `json:"date"`
}

type DiffResult struct {
	Content string
	Error   error
}

// GetCommits returns the commit history for a repository
func GetCommits(gitRepoDir string) ([]CommitInfo, error) {
	repo, err := git.PlainOpen(gitRepoDir)
	if err != nil {
		if err == git.ErrRepositoryNotExists {
			return []CommitInfo{}, nil
		}
		return nil, err
	}

	commits, err := repo.Log(&git.LogOptions{})
	if err != nil {
		return nil, err
	}

	var commitsList []CommitInfo
	err = commits.ForEach(func(c *object.Commit) error {
		commitsList = append(commitsList, CommitInfo{
			Hash: c.Hash.String(),
			Date: c.Author.When,
		})
		return nil
	})

	if err != nil {
		return nil, err
	}

	return commitsList, nil
}

// GetDiff returns the diff for a specific commit
func GetDiff(gitRepoDir string, commitHash string) (*DiffResult, error) {
	repo, err := git.PlainOpen(gitRepoDir)
	if err != nil {
		return nil, err
	}

	hash := plumbing.NewHash(commitHash)
	commit, err := repo.CommitObject(hash)
	if err != nil {
		return nil, err
	}

	parent, err := commit.Parent(0)
	if err != nil && err != object.ErrParentNotFound {
		return nil, err
	}

	currentTree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	var patch *object.Patch
	if parent == nil {
		emptyTree := &object.Tree{}
		patch, err = currentTree.Patch(emptyTree)
	} else {
		parentTree, err := parent.Tree()
		if err != nil {
			return nil, err
		}
		patch, err = parentTree.Patch(currentTree)
	}

	if err != nil {
		return nil, err
	}

	return &DiffResult{Content: patch.String()}, nil
}
