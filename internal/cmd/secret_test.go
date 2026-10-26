package cmd

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/repository"
	"golang.org/x/crypto/nacl/box"

	"github.com/shbernal/gh-envy/internal/cmdutil"
)

// secretAPI serves a fresh environment and its public key, and returns a
// function that decrypts the value PUT for a secret.
func secretAPI(t *testing.T, g *fakeGH, env string) func(name string) string {
	t.Helper()
	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	g.json("GET "+envBase+env, 404, `{"message":"Not Found"}`)
	g.json("PUT "+envBase+env, 200, envJSON(env))
	g.json("GET "+envBase+env+"/secrets/public-key", 200, `{"key_id":"k1","key":"`+base64.StdEncoding.EncodeToString(pub[:])+`"}`)
	g.json("PUT "+envBase+env+"/secrets/{name}", 201, `{}`)
	return func(name string) string {
		t.Helper()
		body := g.one("PUT", envBase+env+"/secrets/"+name).Body
		if body["key_id"] != "k1" {
			t.Errorf("key_id = %v", body["key_id"])
		}
		sealed, _ := base64.StdEncoding.DecodeString(body["encrypted_value"].(string))
		plain, ok := box.OpenAnonymous(nil, sealed, pub, priv)
		if !ok {
			t.Fatalf("%s: cannot open sealed value", name)
		}
		return string(plain)
	}
}

func TestCreateWithSecrets(t *testing.T) {
	g := newFakeGH(t)
	secretOf := secretAPI(t, g, "staging")

	r, _ := run(t, g, true, &stubPrompter{t: t, answers: []any{"s3cret"}}, "create", "staging",
		"--secret", "DB", "--secret", "API_KEY=a=b,c")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.errOut)
	}
	assertContains(t, r.out, "✓ Created environment staging", "✓ Set secret DB", "✓ Set secret API_KEY")
	if got := secretOf("DB"); got != "s3cret" {
		t.Errorf("DB = %q", got)
	}
	if got := secretOf("API_KEY"); got != "a=b,c" {
		t.Errorf("API_KEY = %q", got)
	}
}

func TestCreateSecretFromStdin(t *testing.T) {
	g := newFakeGH(t)
	secretOf := secretAPI(t, g, "staging")

	io, out, errOut := cmdutil.Test(false)
	io.In = bytes.NewBufferString("from-pipe\n")
	repo := repository.Repository{Host: "github.com", Owner: "owner", Name: "repo"}
	f := &cmdutil.Factory{IO: io, Transport: g, Prompter: &stubPrompter{t: t}, Repo: &repo}
	if code := Execute(f, []string{"create", "staging", "--secret", "DB"}); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if out.Len() != 0 {
		t.Errorf("non-tty create printed %q", out)
	}
	if got := secretOf("DB"); got != "from-pipe" {
		t.Errorf("DB = %q", got)
	}
}

func TestCreateSecretValidation(t *testing.T) {
	g := newFakeGH(t)
	g.json("GET "+envBase+"x", 404, `{"message":"Not Found"}`)
	cases := map[string][]string{
		"expected NAME or NAME=VALUE": {"--secret", "=v"},
		"pipe one value":              {"--secret", "A", "--secret", "B"},
	}
	for want, flags := range cases {
		r, _ := run(t, g, false, nil, append([]string{"create", "x"}, flags...)...)
		if r.code != 1 || !strings.Contains(r.errOut, want) {
			t.Errorf("%v: exit %d err %q, want %q", flags, r.code, r.errOut, want)
		}
	}
	if len(g.find("PUT", envBase+"x")) != 0 {
		t.Error("environment created despite invalid --secret")
	}
}
