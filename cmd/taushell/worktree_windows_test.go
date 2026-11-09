//go:build windows

package main

import "testing"

func TestGithubRepo(t *testing.T) {
	for remote, want := range map[string]string{
		"https://github.com/ma99us/tau-shell.git": "ma99us/tau-shell",
		"git@github.com:ma99us/tau-shell.git":     "ma99us/tau-shell",
		"ssh://git@github.com/o/r":                "o/r",
	} {
		owner, repo, ok := githubRepo(remote)
		if !ok || owner+"/"+repo != want {
			t.Errorf("githubRepo(%q) = %q/%q ok=%v", remote, owner, repo, ok)
		}
	}
	for _, remote := range []string{"https://gitlab.com/o/r.git", "git@github.com:o", "C:\\repo"} {
		if _, _, ok := githubRepo(remote); ok {
			t.Errorf("githubRepo(%q) should fail", remote)
		}
	}
}
