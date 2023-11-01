package monorepo_test

import (
	"github.com/garethjevans/monorepository-controller/internal/monorepo"
	"github.com/jenkins-x/go-scm/scm/factory"
	"os"
	"testing"
)

func TestGetClone(t *testing.T) {
	tests := []struct {
		driver         string
		server         string
		repository     string
		token          string
		branch         string
		subPath        string
		previousCommit string
		cloneCommit    string
	}{
		{
			driver:         "github",
			server:         "https://github.com",
			repository:     "garethjevans/monorepository-controller",
			branch:         "main",
			token:          os.Getenv("GITHUB_TOKEN"),
			previousCommit: "7cfc722fc153a4dbd61bc01ae7ec6c8d620cbfed",
			cloneCommit:    "7cfc722fc153a4dbd61bc01ae7ec6c8d620cbfed",
		},
		{
			driver:         "github",
			server:         "https://github.com",
			repository:     "garethjevans/monorepository-controller",
			branch:         "main",
			token:          os.Getenv("GITHUB_TOKEN"),
			previousCommit: "",
			cloneCommit:    "7cfc722fc153a4dbd61bc01ae7ec6c8d620cbfed",
		},
		{
			driver:         "github",
			server:         "https://github.com",
			repository:     "garethjevans/monorepository-controller",
			branch:         "main",
			token:          os.Getenv("GITHUB_TOKEN"),
			previousCommit: "c85a5275030d54543dac63568fd182e726f5e68e",
			cloneCommit:    "7cfc722fc153a4dbd61bc01ae7ec6c8d620cbfed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.repository, func(t *testing.T) {
			if tc.token == "" {
				t.Skip()
			}
			// go scm
			f, err := factory.NewClient(tc.driver, tc.server, tc.token)
			if err != nil {
				t.Fatalf("unable to get client %v", err)
			}

			cloneSha, err := monorepo.DetermineClonePoint(f, tc.repository, tc.branch, tc.previousCommit, tc.subPath)
			if err != nil {
				t.Fatalf("unable to determine clone point %v", err)
			}

			if cloneSha != tc.cloneCommit {
				t.Errorf("expected to clone at sha %s, got %s", tc.cloneCommit, cloneSha)
			}
		})
	}
}
