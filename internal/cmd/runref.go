package cmd

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/cli/go-gh/v2/pkg/repository"
)

// parseRunRef reads a run ID or a run URL
// (https://HOST/OWNER/REPO/actions/runs/ID, optionally followed by /job/N or
// /attempts/N). The repository is set only for URLs.
func parseRunRef(s string) (*repository.Repository, int64, error) {
	s = strings.TrimSpace(s)
	if id, err := strconv.ParseInt(s, 10, 64); err == nil && id > 0 {
		return nil, id, nil
	}
	invalid := fmt.Errorf("invalid run %q: use a run ID or a URL like https://github.com/OWNER/REPO/actions/runs/ID", s)
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, 0, invalid
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	suffixed := len(parts) == 7 && (parts[5] == "job" || parts[5] == "attempts")
	if len(parts) != 5 && !suffixed {
		return nil, 0, invalid
	}
	if parts[0] == "" || parts[1] == "" || parts[2] != "actions" || parts[3] != "runs" {
		return nil, 0, invalid
	}
	id, err := strconv.ParseInt(parts[4], 10, 64)
	if err != nil || id <= 0 {
		return nil, 0, invalid
	}
	if len(parts) == 7 {
		if n, err := strconv.ParseInt(parts[6], 10, 64); err != nil || n <= 0 {
			return nil, 0, invalid
		}
	}
	return &repository.Repository{Host: u.Hostname(), Owner: parts[0], Name: parts[1]}, id, nil
}
