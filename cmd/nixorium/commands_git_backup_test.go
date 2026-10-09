package main

import "testing"

func TestGitBackupArgumentsAndRepositoryFreeRestore(t *testing.T) {
	for _, args := range [][]string{
		{"backup", "clone"},
		{"backup", "plan", "--remote", "git@host:lab.git", "--repo", "/lab", "--json"},
		{"backup", "publish", "--remote", "git@host:lab.git", "--expect", "review", "--yes"},
		{"backup", "clone", "--remote", "git@host:lab.git", "--to", "/new/lab", "--yes"},
	} {
		o, err := parseArguments(args)
		if err != nil {
			t.Fatal(args, err)
		}
		if o.subcommand == "clone" && commandRequiresRepository(o) {
			t.Fatal("restore requires existing deployment")
		}
	}
	for _, args := range [][]string{
		{"backup", "publish", "--remote", "git@host:lab.git", "--yes"},
		{"backup", "publish", "--remote", "git@host:lab.git", "--expect", "review"},
		{"backup", "clone", "--remote", "git@host:lab.git", "--to", "/new/lab"},
		{"backup", "plan", "--remote", "git@host:lab.git", "--passphrase-file", "/secret"},
	} {
		if _, err := parseArguments(args); err == nil {
			t.Fatal("unsafe arguments accepted", args)
		}
	}
}
