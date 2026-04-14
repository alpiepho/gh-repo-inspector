package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// Repo holds metadata for a single GitHub repository.
type Repo struct {
	Name        string
	Description string
	PushedAt    time.Time
	IsPrivate   bool
	IsFork      bool
	DiskUsage   int // KB — total across all branches/history
	URL         string
	BranchCount int
}

// graphQL query — fetches all fields including branch count, paginated.
const repoQuery = `query ListRepos($endCursor: String) {
  viewer {
    repositories(first: 100, after: $endCursor) {
      pageInfo { hasNextPage endCursor }
      nodes {
        name
        description
        pushedAt
        isPrivate
        isFork
        diskUsage
        url
        refs(refPrefix: "refs/heads/") { totalCount }
      }
    }
  }
}`

type gqlResponse struct {
	Data struct {
		Viewer struct {
			Repositories struct {
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []struct {
					Name        string    `json:"name"`
					Description string    `json:"description"`
					PushedAt    time.Time `json:"pushedAt"`
					IsPrivate   bool      `json:"isPrivate"`
					IsFork      bool      `json:"isFork"`
					DiskUsage   int       `json:"diskUsage"`
					URL         string    `json:"url"`
					Refs        struct {
						TotalCount int `json:"totalCount"`
					} `json:"refs"`
				} `json:"nodes"`
			} `json:"repositories"`
		} `json:"viewer"`
	} `json:"data"`
}

// ListRepos fetches all repos for the authenticated user via the gh CLI GraphQL API.
// DiskUsage reflects the full repository size (all branches and history).
func ListRepos() ([]Repo, error) {
	var all []Repo
	cursor := ""

	for {
		args := []string{"api", "graphql", "-f", "query=" + repoQuery}
		if cursor != "" {
			args = append(args, "-f", "endCursor="+cursor)
		}
		out, err := exec.Command("gh", args...).Output()
		if err != nil {
			return nil, fmt.Errorf("gh api graphql: %w", err)
		}

		var resp gqlResponse
		if err := json.Unmarshal(out, &resp); err != nil {
			return nil, fmt.Errorf("parse graphql response: %w", err)
		}

		repos := resp.Data.Viewer.Repositories
		for _, n := range repos.Nodes {
			all = append(all, Repo{
				Name:        n.Name,
				Description: n.Description,
				PushedAt:    n.PushedAt,
				IsPrivate:   n.IsPrivate,
				IsFork:      n.IsFork,
				DiskUsage:   n.DiskUsage,
				URL:         n.URL,
				BranchCount: n.Refs.TotalCount,
			})
		}

		if !repos.PageInfo.HasNextPage {
			break
		}
		cursor = repos.PageInfo.EndCursor
	}

	return all, nil
}

// DeleteRepo deletes a repository. In dry-run mode it returns nil without acting.
func DeleteRepo(fullName string, dryRun bool) error {
	if dryRun {
		return nil
	}
	cmd := exec.Command("gh", "repo", "delete", fullName, "--yes")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gh repo delete: %s: %w", out, err)
	}
	return nil
}

// SetPrivate changes a repository's visibility to private.
// In dry-run mode it returns nil without acting.
func SetPrivate(fullName string, dryRun bool) error {
	if dryRun {
		return nil
	}
	cmd := exec.Command("gh", "repo", "edit", fullName, "--visibility", "private")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gh repo edit: %s: %w", out, err)
	}
	return nil
}

// CloneRepo clones a repository into destDir.
// Pass allBranches=false for --single-branch (default branch only, faster).
// Pass allBranches=true for a full clone (all remote-tracking branches).
// In dry-run mode it returns nil without acting.
// ctx can carry a deadline to prevent hangs.
func CloneRepo(ctx context.Context, url, destDir string, allBranches, dryRun bool) error {
	if dryRun {
		return nil
	}
	args := []string{"clone"}
	if !allBranches {
		args = append(args, "--single-branch")
	}
	args = append(args, url, destDir)
	cmd := exec.CommandContext(ctx, "git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone: %s: %w", out, err)
	}
	return nil
}

// FormatSize converts KB to a human-readable string.
func FormatSize(kb int) string {
	switch {
	case kb >= 1024*1024:
		return fmt.Sprintf("%.1f GB", float64(kb)/1024/1024)
	case kb >= 1024:
		return fmt.Sprintf("%.1f MB", float64(kb)/1024)
	default:
		return fmt.Sprintf("%d KB", kb)
	}
}
