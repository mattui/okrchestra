package metrics

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type GitProvider struct {
	RepoDir string
	AsOf    time.Time
}

func (p *GitProvider) Name() string { return "git" }

func (p *GitProvider) Collect(ctx context.Context) ([]MetricPoint, error) {
	asOf := p.AsOf.UTC().Truncate(24 * time.Hour)
	until := asOf.Add(24 * time.Hour)
	since := until.Add(-30 * 24 * time.Hour)

	commits, err := gitCount(ctx, p.RepoDir, []string{
		"rev-list",
		"--count",
		"--since=" + since.Format(time.RFC3339),
		"--until=" + until.Format(time.RFC3339),
		"HEAD",
	})
	if err != nil {
		return nil, err
	}
	mergeCommits, err := gitCount(ctx, p.RepoDir, []string{
		"rev-list",
		"--count",
		"--merges",
		"--since=" + since.Format(time.RFC3339),
		"--until=" + until.Format(time.RFC3339),
		"HEAD",
	})
	if err != nil {
		return nil, err
	}

	ts := AsOfTimestamp(asOf)
	return []MetricPoint{
		{
			Key:       "git.commits_30d",
			Value:     float64(commits),
			Unit:      "count",
			Timestamp: ts,
			Source:    p.Name(),
		},
		{
			Key:       "git.merge_commits_30d",
			Value:     float64(mergeCommits),
			Unit:      "count",
			Timestamp: ts,
			Source:    p.Name(),
		},
	}, nil
}

func gitCount(ctx context.Context, dir string, args []string) (int64, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return 0, fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), msg, err)
		}
		return 0, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	raw := strings.TrimSpace(stdout.String())
	if raw == "" {
		return 0, fmt.Errorf("git %s returned empty output", strings.Join(args, " "))
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse git output %q: %w", raw, err)
	}
	return v, nil
}
