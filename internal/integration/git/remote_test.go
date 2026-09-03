package git

import "testing"

func TestNormalizeRemoteEquivalentForms(t *testing.T) {
	want := RemoteIdentity{Host: "github.com", Owner: "org", Repository: "repo"}
	for _, remote := range []string{
		"git@github.com:org/repo.git",
		"https://github.com/org/repo.git",
		"https://github.com/org/repo",
		"ssh://git@github.com/org/repo.git",
		"https://GITHUB.COM/org/repo.git/",
	} {
		t.Run(remote, func(t *testing.T) {
			got, err := NormalizeRemote(remote)
			if err != nil {
				t.Fatalf("NormalizeRemote() error = %v", err)
			}
			if got != want {
				t.Errorf("NormalizeRemote() = %#v, want %#v", got, want)
			}
		})
	}
}

func TestNormalizeRemoteEnterprise(t *testing.T) {
	got, err := NormalizeRemote("git@git.example.com:team/backend.git")
	if err != nil {
		t.Fatalf("NormalizeRemote() error = %v", err)
	}
	want := RemoteIdentity{Host: "git.example.com", Owner: "team", Repository: "backend"}
	if got != want {
		t.Errorf("NormalizeRemote() = %#v, want %#v", got, want)
	}
}

func TestNormalizeRemoteRejectsInvalid(t *testing.T) {
	for _, remote := range []string{"", "github.com/org/repo", "https://github.com/org", "https:///org/repo"} {
		if _, err := NormalizeRemote(remote); err == nil {
			t.Errorf("NormalizeRemote(%q) error = nil, want invalid remote", remote)
		}
	}
}
