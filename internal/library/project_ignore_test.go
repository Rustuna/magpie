package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dotGit makes dir the root of a repository without running git: a .git
// folder whose exclude file holds a line of the user's. It is the file.
func dotGit(t *testing.T, dir string) string {
	t.Helper()
	f := filepath.Join(dir, ".git", "info", "exclude")
	write(t, f, "*.log\n")
	return f
}

// linkedWorktree makes dir a linked worktree of the repository at main, as
// `git worktree add` lays it out: a .git file naming the worktree's folder
// under main's .git, whose commondir points back at the clone's.
func linkedWorktree(t *testing.T, main, dir, name string) {
	t.Helper()
	wt := filepath.Join(main, ".git", "worktrees", name)
	write(t, filepath.Join(wt, "commondir"), "../..\n")
	write(t, filepath.Join(wt, "gitdir"), filepath.Join(dir, ".git")+"\n")
	write(t, filepath.Join(dir, ".git"), "gitdir: "+wt+"\n")
}

// #5670: a project in a repository whose .gitignore is tracked can have
// magpie's lines in the repository's exclude file instead, which git never
// tracks. The .gitignore is left as the user's.
func TestProjectIgnoreInExcludeFile(t *testing.T) {
	_, proj := projectLib(t)
	excl := dotGit(t, proj)
	ok(t)(ProjectIgnore(proj, IgnoreInExclude))
	ok(t)(ProjectSkill(proj, "pdf", []string{"claude", "codex"}))
	if g := read(t, filepath.Join(proj, ".gitignore")); g != "node_modules/\n" {
		t.Errorf(".gitignore was written: %q", g)
	}
	want := "*.log\n\n" + ignoreBegin + "\n/.agents/skills/pdf\n/.claude/skills/pdf\n" + ignoreEnd + "\n"
	if g := read(t, excl); g != want {
		t.Errorf("exclude file:\n%s", g)
	}
	for _, e := range []string{".claude/skills/pdf", ".agents/skills/pdf"} {
		if !isLink(t, filepath.Join(proj, e)) {
			t.Errorf("%s isn't placed", e)
		}
	}
	v, _ := Read(nil)
	if len(v.Projects) != 1 || v.Projects[0].Ignore != IgnoreInExclude || !v.Projects[0].Git {
		t.Errorf("projects: %+v", v.Projects)
	}

	// the skill taken out: magpie's lines go, the user's stay
	ok(t)(ProjectSkill(proj, "pdf", nil))
	if g := read(t, excl); g != "*.log\n" {
		t.Errorf("exclude file after: %q", g)
	}
	gone(t, filepath.Join(proj, ".claude/skills/pdf"))
}

func TestProjectIgnoreMovesWithTheChoice(t *testing.T) {
	_, proj := projectLib(t)
	excl := dotGit(t, proj)
	gi := filepath.Join(proj, ".gitignore")
	ok(t)(ProjectSkill(proj, "pdf", []string{"claude"}))
	inGitignore := "node_modules/\n\n" + ignoreBegin + "\n/.claude/skills/pdf\n" + ignoreEnd + "\n"
	if g := read(t, gi); g != inGitignore {
		t.Fatalf(".gitignore: %q", g)
	}

	ok(t)(ProjectIgnore(proj, IgnoreInExclude))
	if g := read(t, gi); g != "node_modules/\n" {
		t.Errorf(".gitignore keeps the lines: %q", g)
	}
	if g := read(t, excl); g != "*.log\n\n"+ignoreBegin+"\n/.claude/skills/pdf\n"+ignoreEnd+"\n" {
		t.Errorf("exclude file: %q", g)
	}

	ok(t)(ProjectIgnore(proj, IgnoreNowhere))
	if g := read(t, excl); g != "*.log\n" {
		t.Errorf("exclude file keeps the lines: %q", g)
	}
	if g := read(t, gi); g != "node_modules/\n" {
		t.Errorf(".gitignore: %q", g)
	}
	if !isLink(t, filepath.Join(proj, ".claude/skills/pdf")) {
		t.Error("the skill went with the lines")
	}

	ok(t)(ProjectIgnore(proj, IgnoreInGitignore))
	if g := read(t, gi); g != inGitignore {
		t.Errorf(".gitignore: %q", g)
	}
	if g := read(t, excl); g != "*.log\n" {
		t.Errorf("exclude file: %q", g)
	}
}

// A project folder below the repository's root: the exclude file's lines
// are read from the root, so they name the folder.
func TestProjectIgnoreBelowTheRepositoryRoot(t *testing.T) {
	_, proj := projectLib(t)
	excl := dotGit(t, filepath.Dir(proj))
	ok(t)(ProjectIgnore(proj, IgnoreInExclude))
	ok(t)(ProjectSkill(proj, "pdf", []string{"codex"}))
	want := "*.log\n\n" + ignoreBegin + "\n/" + filepath.Base(proj) + "/.agents/skills/pdf\n" + ignoreEnd + "\n"
	if g := read(t, excl); g != want {
		t.Errorf("exclude file:\n%s", g)
	}
	ok(t)(ProjectSkill(proj, "pdf", nil))
	if g := read(t, excl); g != "*.log\n" {
		t.Errorf("exclude file after: %q", g)
	}
}

// A linked worktree's .git is a file naming its folder under the clone's
// .git; the exclude file is the clone's, shared by every worktree of it.
func TestProjectIgnoreInLinkedWorktree(t *testing.T) {
	_, proj := projectLib(t)
	main := filepath.Join(t.TempDir(), "main")
	excl := dotGit(t, main)
	linkedWorktree(t, main, proj, "feature")
	ok(t)(ProjectIgnore(proj, IgnoreInExclude))
	ok(t)(ProjectSkill(proj, "docx", []string{"claude"}))
	if g := read(t, excl); g != "*.log\n\n"+ignoreBegin+"\n/.claude/skills/docx\n"+ignoreEnd+"\n" {
		t.Errorf("the clone's exclude file: %q", g)
	}
	if _, err := os.Stat(filepath.Join(main, ".git", "worktrees", "feature", "info")); !os.IsNotExist(err) {
		t.Error("the worktree's own folder got an exclude file")
	}
	if g := read(t, filepath.Join(proj, ".gitignore")); g != "node_modules/\n" {
		t.Errorf(".gitignore: %q", g)
	}
}

// The exclude file is made when the repository has none yet (an empty
// template), and keeps the line endings it has.
func TestProjectIgnoreExcludeFileMadeAndCRLF(t *testing.T) {
	_, proj := projectLib(t)
	if err := os.MkdirAll(filepath.Join(proj, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	excl := filepath.Join(proj, ".git", "info", "exclude")
	ok(t)(ProjectIgnore(proj, IgnoreInExclude))
	ok(t)(ProjectSkill(proj, "pdf", []string{"claude"}))
	if g := read(t, excl); g != ignoreBegin+"\n/.claude/skills/pdf\n"+ignoreEnd+"\n" {
		t.Errorf("exclude file made: %q", g)
	}
	// the repository's file stays once magpie's lines are out: it isn't magpie's
	ok(t)(ProjectSkill(proj, "pdf", nil))
	if g := read(t, excl); g != "" {
		t.Errorf("exclude file after: %q", g)
	}
	if _, err := os.Stat(excl); err != nil {
		t.Errorf("the exclude file went: %v", err)
	}

	write(t, excl, "*.log\r\n")
	ok(t)(ProjectSkill(proj, "pdf", []string{"claude"}))
	if g := read(t, excl); g != "*.log\r\n\r\n"+ignoreBegin+"\r\n/.claude/skills/pdf\r\n"+ignoreEnd+"\r\n" {
		t.Errorf("exclude file with CRLF: %q", g)
	}
}

func TestProjectIgnoreExcludeNeedsARepository(t *testing.T) {
	_, proj := projectLib(t)
	if _, err := ProjectIgnore(proj, IgnoreInExclude); err == nil || !strings.Contains(err.Error(), "isn't in a git repository") {
		t.Errorf("err = %v", err)
	}
	if _, err := ProjectIgnore(proj, "elsewhere"); err == nil {
		t.Error("an unknown place was taken")
	}
	v, _ := Read(nil)
	if v.Projects[0].Ignore != IgnoreInGitignore || v.Projects[0].Git {
		t.Errorf("project: %+v", v.Projects[0])
	}

	// a repository gone after the choice: the sync says so, places the
	// skill, and writes the lines nowhere rather than into the .gitignore
	dotGit(t, proj)
	ok(t)(ProjectIgnore(proj, IgnoreInExclude))
	if err := os.RemoveAll(filepath.Join(proj, ".git")); err != nil {
		t.Fatal(err)
	}
	r, err := ProjectSkill(proj, "pdf", []string{"claude"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Problems) != 1 || r.Problems[0].Agent != proj || !strings.Contains(r.Problems[0].Error, "isn't in a git repository") {
		t.Errorf("problems: %+v", r.Problems)
	}
	if g := read(t, filepath.Join(proj, ".gitignore")); g != "node_modules/\n" {
		t.Errorf(".gitignore: %q", g)
	}
	if !isLink(t, filepath.Join(proj, ".claude/skills/pdf")) {
		t.Error("the skill wasn't placed")
	}
}

// Lines nowhere: the user's own rules cover the folders, and no .gitignore
// is made for the lines.
func TestProjectIgnoreNowhere(t *testing.T) {
	_, proj := projectLib(t)
	os.Remove(filepath.Join(proj, ".gitignore"))
	ok(t)(ProjectIgnore(proj, IgnoreNowhere))
	ok(t)(ProjectSkill(proj, "docx", []string{"claude"}))
	gone(t, filepath.Join(proj, ".gitignore"))
	if !isLink(t, filepath.Join(proj, ".claude/skills/docx")) {
		t.Error("the skill wasn't placed")
	}
	v, _ := Read(nil)
	if v.Projects[0].Ignore != IgnoreNowhere {
		t.Errorf("project: %+v", v.Projects[0])
	}
}

// A server's file magpie makes is listed where the skills' lines go.
func TestProjectIgnoreListsMadeFilesInExclude(t *testing.T) {
	_, proj := mcpProject(t)
	excl := dotGit(t, proj)
	ok(t)(ProjectIgnore(proj, IgnoreInExclude))
	ok(t)(ProjectServer(proj, "fs", []string{"codex"}))
	if g := read(t, excl); g != "*.log\n\n"+ignoreBegin+"\n/.codex/config.toml\n"+ignoreEnd+"\n" {
		t.Errorf("exclude file: %q", g)
	}
	if g := read(t, filepath.Join(proj, ".gitignore")); strings.Contains(g, ignoreBegin) {
		t.Errorf(".gitignore: %q", g)
	}
}

// Removing the project takes its lines out of the exclude file as it does
// out of a .gitignore; kept, they stay, the user's from then on.
func TestRemoveProjectAndTheExcludeFile(t *testing.T) {
	_, proj := projectLib(t)
	excl := dotGit(t, proj)
	ok(t)(ProjectIgnore(proj, IgnoreInExclude))
	ok(t)(ProjectSkill(proj, "pdf", []string{"claude"}))
	ok(t)(RemoveProject(proj, false))
	if g := read(t, excl); g != "*.log\n" {
		t.Errorf("exclude file: %q", g)
	}
	gone(t, filepath.Join(proj, ".claude/skills/pdf"))

	_, proj = projectLib(t)
	excl = dotGit(t, proj)
	ok(t)(ProjectIgnore(proj, IgnoreInExclude))
	ok(t)(ProjectSkill(proj, "pdf", []string{"claude"}))
	ok(t)(RemoveProject(proj, true))
	if g := read(t, excl); !strings.Contains(g, "/.claude/skills/pdf") {
		t.Errorf("exclude file: %q", g)
	}
	if !isLink(t, filepath.Join(proj, ".claude/skills/pdf")) {
		t.Error("the kept skill went")
	}
}
