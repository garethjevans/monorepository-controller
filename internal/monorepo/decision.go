package monorepo

import (
	"context"
	"fmt"
	"github.com/jenkins-x/go-scm/scm"
	"strings"
)

func DetermineClonePoint(client *scm.Client, repository string, branch string, previousCommit string, subPath string) (string, error) {
	ctx := context.Background()

	ref, _, err := client.Git.FindBranch(ctx, repository, branch)
	if err != nil {
		return "", err
	}

	latestCommitOnBranch := ref.Sha

	// this is the first time we are seeing this repository, so we need to clone it all
	if previousCommit == "" {
		return latestCommitOnBranch, nil
	}

	// our understanding of the repository is up to date, so there is no work to be done
	if latestCommitOnBranch == previousCommit {
		return previousCommit, nil
	}

	// if subPath is not set, we want the whole repository
	if subPath == "" {
		return latestCommitOnBranch, nil
	}

	changes, _, err := client.Git.CompareCommits(ctx, repository, previousCommit, latestCommitOnBranch, &scm.ListOptions{})
	if err != nil {
		return "", err
	}

	for _, change := range changes {
		fmt.Printf("%s\n", change.Path)
		if strings.HasPrefix(change.Path, subPath) {
			return latestCommitOnBranch, nil
		}
	}

	return previousCommit, nil
}
