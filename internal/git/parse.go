package git

import (
	"fmt"
	"strings"
)

// parseWorktreeList parses the output of `git worktree list --porcelain`.
// Records are separated by blank lines; each record starts with a "worktree" line.
func parseWorktreeList(output string) ([]WorktreeInfo, error) {
	var worktrees []WorktreeInfo
	var cur *WorktreeInfo

	for line := range strings.SplitSeq(output, "\n") {
		switch {
		case line == "":
			if cur != nil {
				worktrees = append(worktrees, *cur)
				cur = nil
			}
		case strings.HasPrefix(line, "worktree "):
			cur = &WorktreeInfo{Path: strings.TrimPrefix(line, "worktree ")}
		case cur == nil:
			// ignore lines before the first worktree record
		case strings.HasPrefix(line, "HEAD "):
			cur.HEAD = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "bare":
			cur.Bare = true
		case line == "locked" || strings.HasPrefix(line, "locked "):
			cur.Locked = true
		}
	}

	// Flush the final record (output may not end with a blank line).
	if cur != nil {
		worktrees = append(worktrees, *cur)
	}

	return worktrees, nil
}

// parseStatus parses the output of `git status --porcelain=v2 --branch`.
func parseStatus(output string) (RepoStatus, error) {
	var s RepoStatus

	for line := range strings.SplitSeq(output, "\n") {
		if line == "" {
			continue
		}

		switch {
		case strings.HasPrefix(line, "# branch.head "):
			name := strings.TrimPrefix(line, "# branch.head ")
			if name != "(detached)" {
				s.Branch = name
			}

		case strings.HasPrefix(line, "# branch.upstream "):
			s.Upstream = strings.TrimPrefix(line, "# branch.upstream ")

		case strings.HasPrefix(line, "# branch.ab "):
			if _, err := fmt.Sscanf(strings.TrimPrefix(line, "# branch.ab "), "+%d -%d", &s.Ahead, &s.Behind); err != nil {
				return s, fmt.Errorf("parse branch.ab %q: %w", line, err)
			}

		case len(line) > 1 && line[0] == '1' && line[1] == ' ':
			// Changed tracked file: 1 XY sub mH mI mW hH hI path
			parts := strings.SplitN(line, " ", 9)
			if len(parts) == 9 && len(parts[1]) == 2 {
				s.Files = append(s.Files, FileStatus{
					Path:     parts[8],
					Index:    parts[1][0],
					Worktree: parts[1][1],
				})
			}

		case len(line) > 1 && line[0] == '2' && line[1] == ' ':
			// Renamed/copied: 2 XY sub mH mI mW hH hI X<score> newPath\torigPath
			parts := strings.SplitN(line, " ", 10)
			if len(parts) == 10 && len(parts[1]) == 2 {
				newPath, _, _ := strings.Cut(parts[9], "\t")
				s.Files = append(s.Files, FileStatus{
					Path:     newPath,
					Index:    parts[1][0],
					Worktree: parts[1][1],
				})
			}

		case len(line) > 1 && line[0] == 'u' && line[1] == ' ':
			// Unmerged: u XY sub m1 m2 m3 mW h1 h2 h3 path
			parts := strings.SplitN(line, " ", 11)
			if len(parts) == 11 && len(parts[1]) == 2 {
				s.Files = append(s.Files, FileStatus{
					Path:     parts[10],
					Index:    parts[1][0],
					Worktree: parts[1][1],
				})
			}

		case len(line) > 1 && line[0] == '?' && line[1] == ' ':
			s.Files = append(s.Files, FileStatus{
				Path:     line[2:],
				Index:    '?',
				Worktree: '?',
			})
		}
	}

	return s, nil
}
