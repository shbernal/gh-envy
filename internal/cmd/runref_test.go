package cmd

import (
	"testing"

	"github.com/cli/go-gh/v2/pkg/repository"
)

func TestParseRunRef(t *testing.T) {
	gh := &repository.Repository{Host: "github.com", Owner: "owner", Name: "repo"}
	tests := []struct {
		in      string
		repo    *repository.Repository
		id      int64
		wantErr bool
	}{
		{in: "123", id: 123},
		{in: " 123 ", id: 123},
		{in: "https://github.com/owner/repo/actions/runs/123", repo: gh, id: 123},
		{in: "https://github.com/owner/repo/actions/runs/123/", repo: gh, id: 123},
		{in: "https://github.com/owner/repo/actions/runs/123/job/456", repo: gh, id: 123},
		{in: "https://github.com/owner/repo/actions/runs/123/attempts/2", repo: gh, id: 123},
		{in: "https://ghe.example.com/acme/app/actions/runs/9", repo: &repository.Repository{Host: "ghe.example.com", Owner: "acme", Name: "app"}, id: 9},
		{in: "0", wantErr: true},
		{in: "-5", wantErr: true},
		{in: "abc", wantErr: true},
		{in: "", wantErr: true},
		{in: "github.com/owner/repo/actions/runs/123", wantErr: true},
		{in: "https://github.com/owner/repo/actions/runs/abc", wantErr: true},
		{in: "https://github.com/owner/repo/actions/runs", wantErr: true},
		{in: "https://github.com/owner/repo/pull/123", wantErr: true},
		{in: "https://github.com/owner/repo/actions/runs/123/logs/4", wantErr: true},
		{in: "https://github.com/owner/repo/actions/runs/123/job/x", wantErr: true},
		{in: "https://github.com/owner/repo/actions/runs/123/job/4/extra", wantErr: true},
	}
	for _, tt := range tests {
		repo, id, err := parseRunRef(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("%q: want error, got %v %d", tt.in, repo, id)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", tt.in, err)
			continue
		}
		if id != tt.id || (repo == nil) != (tt.repo == nil) || (repo != nil && *repo != *tt.repo) {
			t.Errorf("%q: got %v %d, want %v %d", tt.in, repo, id, tt.repo, tt.id)
		}
	}
}
