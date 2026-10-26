package cmdutil

import (
	"errors"
	"net/http"
	"os"

	ghapi "github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/browser"
	"github.com/cli/go-gh/v2/pkg/prompter"
	"github.com/cli/go-gh/v2/pkg/repository"

	"github.com/shbernal/gh-envy/internal/api"
)

// Prompter is the subset of go-gh's prompter the commands use.
type Prompter interface {
	Select(prompt, defaultValue string, options []string) (int, error)
	Input(prompt, defaultValue string) (string, error)
	Password(prompt string) (string, error)
	Confirm(prompt string, defaultValue bool) (bool, error)
}

// Browser opens URLs.
type Browser interface {
	Browse(url string) error
}

// Factory builds the dependencies of a command run.
type Factory struct {
	IO        *IO
	RepoFlag  string
	Transport http.RoundTripper
	Prompter  Prompter
	Browser   Browser
	// Repo overrides repository resolution, for tests.
	Repo *repository.Repository
}

// NewFactory returns a Factory for a real terminal session.
func NewFactory() *Factory {
	io := System()
	return &Factory{
		IO:       io,
		Prompter: prompter.New(os.Stdin, os.Stdout, os.Stderr),
		Browser:  browser.New("", io.Out, io.ErrOut),
	}
}

// BaseRepo resolves -R, then GH_REPO, then the current git remote.
func (f *Factory) BaseRepo() (repository.Repository, error) {
	return f.ParseRepo(f.RepoFlag)
}

// ParseRepo resolves an [HOST/]OWNER/REPO argument, or the current repo when empty.
func (f *Factory) ParseRepo(s string) (repository.Repository, error) {
	if s != "" {
		return repository.Parse(s)
	}
	if f.Repo != nil {
		return *f.Repo, nil
	}
	repo, err := repository.Current()
	if err != nil {
		return repository.Repository{}, errors.New("could not determine the repository: run inside a GitHub checkout or pass -R OWNER/REPO")
	}
	return repo, nil
}

// Client returns an API client for the base repository.
func (f *Factory) Client() (*api.Client, error) {
	repo, err := f.BaseRepo()
	if err != nil {
		return nil, err
	}
	opts := ghapi.ClientOptions{Host: repo.Host}
	if f.Transport != nil {
		opts.AuthToken = "test-token"
		opts.Transport = f.Transport
	}
	return api.New(repo, opts)
}
