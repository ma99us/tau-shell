// Package worktree manages git worktrees for sessions: it inspects a folder,
// creates a sibling worktree on a new branch, copies ignored files listed in
// .worktreeinclude, and removes a worktree safely. Git runs natively or inside
// the WSL distro carried by the context (see presets.WithWSL).
package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ma99us/tau-shell/internal/presets"
)

// IncludeFile lists gitignore-style patterns of ignored files (such as .env)
// that are copied into every new worktree.
const IncludeFile = ".worktreeinclude"

// MaxPathLength warns about paths that risk the Windows MAX_PATH limit.
const MaxPathLength = 200

// BranchPrefix starts the default branch name of a TauShell worktree.
const BranchPrefix = "taushell/"

var (
	namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,47}$`)
	slugPattern = regexp.MustCompile(`[^a-z0-9._-]+`)
	// Names Windows cannot use for a folder.
	reservedNames = map[string]bool{"con": true, "prn": true, "aux": true, "nul": true,
		"com1": true, "com2": true, "com3": true, "com4": true, "com5": true, "com6": true, "com7": true, "com8": true, "com9": true,
		"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true, "lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true}
)

// Info describes the folder a session works in when it is a linked worktree.
type Info struct {
	// Name is the worktree folder name, shown in place of the branch.
	Name string `json:"name"`
	// Path is the worktree root and MainRepo the main working tree.
	Path     string `json:"path"`
	MainRepo string `json:"mainRepo"`
	Branch   string `json:"branch"`
}

// Slug turns free text into a worktree name: lowercase letters, digits, dots,
// dashes and underscores. It returns "" when nothing usable is left.
func Slug(text string) string {
	name := slugPattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(text)), "-")
	name = strings.Trim(name, "-._")
	if len(name) > 48 {
		name = strings.Trim(name[:48], "-._")
	}
	return name
}

// ValidateName checks a worktree folder name.
func ValidateName(name string) error {
	switch {
	case !namePattern.MatchString(name):
		return fmt.Errorf("worktree name %q must be 1 to 48 lowercase letters, digits, dots, dashes or underscores, starting with a letter or digit", name)
	case strings.Contains(name, ".."), strings.HasSuffix(name, "."), strings.HasSuffix(name, ".lock"):
		return fmt.Errorf("worktree name %q is not a valid folder and branch name", name)
	case reservedNames[strings.SplitN(name, ".", 2)[0]]:
		return fmt.Errorf("worktree name %q is reserved by Windows", name)
	}
	return nil
}

// DefaultBranch is the suggested branch for a worktree name.
func DefaultBranch(name string) string { return BranchPrefix + name }

// ValidateBranch checks a branch name with git's own rules.
func ValidateBranch(ctx context.Context, repo, branch string) error {
	if strings.TrimSpace(branch) == "" || strings.HasPrefix(branch, "-") {
		return fmt.Errorf("branch name %q is not valid", branch)
	}
	if _, err := presets.RunGit(ctx, repo, nil, "check-ref-format", "--branch", branch); err != nil {
		return fmt.Errorf("branch name %q is not valid", branch)
	}
	return nil
}

func linux(ctx context.Context) bool {
	_, inWSL := presets.WSLDistro(ctx)
	return inWSL
}

// join, parent and base use the separators of the machine git runs on.
func join(ctx context.Context, elements ...string) string {
	if linux(ctx) {
		return pathpkg.Join(elements...)
	}
	return filepath.Join(elements...)
}

func parent(ctx context.Context, path string) string {
	if linux(ctx) {
		return pathpkg.Dir(path)
	}
	return filepath.Dir(path)
}

func base(ctx context.Context, path string) string {
	if linux(ctx) {
		return pathpkg.Base(path)
	}
	return filepath.Base(path)
}

// native converts a path printed by git into the form of the machine it runs on.
func native(ctx context.Context, path string) string {
	path = strings.TrimSpace(path)
	if linux(ctx) || path == "" {
		return path
	}
	return filepath.Clean(filepath.FromSlash(path))
}

func sameDir(ctx context.Context, a, b string) bool {
	if linux(ctx) {
		return pathpkg.Clean(a) == pathpkg.Clean(b)
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// Inspect reports whether dir is inside a linked worktree (not the main
// working tree and not a submodule). It runs git, so it is not free.
func Inspect(ctx context.Context, dir string) (Info, bool) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	out, err := presets.RunGit(ctx, dir, nil, "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-dir", "--git-common-dir", "--abbrev-ref", "HEAD")
	if err != nil {
		return Info{}, false
	}
	return parseInspect(ctx, out)
}

func parseInspect(ctx context.Context, output string) (Info, bool) {
	lines := strings.Split(strings.ReplaceAll(output, "\r", ""), "\n")
	if len(lines) < 4 {
		return Info{}, false
	}
	top, gitDir, common := native(ctx, lines[0]), native(ctx, lines[1]), native(ctx, lines[2])
	if top == "" || sameDir(ctx, gitDir, common) {
		return Info{}, false
	}
	main := common
	if strings.EqualFold(base(ctx, common), ".git") {
		main = parent(ctx, common)
	}
	branch := strings.TrimSpace(lines[3])
	if branch == "HEAD" {
		branch = "detached"
	}
	return Info{Name: base(ctx, top), Path: top, MainRepo: main, Branch: branch}, true
}

// Root returns the main working tree of the repository containing dir, whether
// dir is the main checkout or a linked worktree.
func Root(ctx context.Context, dir string) (string, error) {
	if info, ok := Inspect(ctx, dir); ok {
		return info.MainRepo, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	out, err := presets.RunGit(ctx, dir, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", errors.New("the project folder is not a git repository")
	}
	return native(ctx, out), nil
}

// Entry is one worktree of a repository.
type Entry struct {
	Path     string `json:"path"`
	Branch   string `json:"branch"`
	Main     bool   `json:"main"`
	Detached bool   `json:"detached"`
	Locked   bool   `json:"locked"`
	Prunable bool   `json:"prunable"`
}

// List returns the worktrees of the repository containing dir; the first
// entry is the main working tree.
func List(ctx context.Context, dir string) ([]Entry, error) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	out, err := presets.RunGit(ctx, dir, nil, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parseList(ctx, out), nil
}

func parseList(ctx context.Context, output string) []Entry {
	var entries []Entry
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r", ""), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			entries = append(entries, Entry{Path: native(ctx, strings.TrimPrefix(line, "worktree ")), Main: len(entries) == 0})
			continue
		}
		if len(entries) == 0 {
			continue
		}
		current := &entries[len(entries)-1]
		switch {
		case strings.HasPrefix(line, "branch "):
			current.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "detached":
			current.Detached = true
		case strings.HasPrefix(line, "locked"):
			current.Locked = true
		case strings.HasPrefix(line, "prunable"):
			current.Prunable = true
		}
	}
	return entries
}

// Branches lists local and remote branches (without HEAD) as base choices,
// with the current branch first.
func Branches(ctx context.Context, dir string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	out, err := presets.RunGit(ctx, dir, nil, "for-each-ref", "--format=%(refname:short)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	current, _ := presets.RunGit(ctx, dir, nil, "rev-parse", "--abbrev-ref", "HEAD")
	current = strings.TrimSpace(current)
	var result []string
	if current != "" && current != "HEAD" {
		result = append(result, current)
	}
	for _, name := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		name = strings.TrimSpace(name)
		if name == "" || name == current || strings.HasSuffix(name, "/HEAD") || name == "origin" {
			continue
		}
		result = append(result, name)
	}
	return result, nil
}

// Spec describes a new worktree.
type Spec struct {
	// Dir is any folder of the repository; the worktree goes next to the main working tree.
	Dir    string
	Name   string
	Branch string
	// Base is the commit-ish the branch starts from; empty means the current HEAD.
	Base string
}

// Result is what Create produced.
type Result struct {
	Path     string   `json:"path"`
	MainRepo string   `json:"mainRepo"`
	Copied   int      `json:"copied"`
	Warnings []string `json:"warnings,omitempty"`
}

// PlannedPath is where a worktree called name would be created, so the dialog
// can show it before anything happens.
func PlannedPath(ctx context.Context, dir, name string) (string, error) {
	main, err := Root(ctx, dir)
	if err != nil {
		return "", err
	}
	return planned(ctx, main, name), nil
}

func planned(ctx context.Context, main, name string) string {
	return join(ctx, parent(ctx, main), base(ctx, main)+".worktrees", name)
}

// Create adds a worktree on a new branch and copies the ignored files listed
// in .worktreeinclude. Copy problems are warnings, not failures; any failure
// of git itself leaves nothing behind.
func Create(ctx context.Context, spec Spec) (Result, error) {
	if err := ValidateName(spec.Name); err != nil {
		return Result{}, err
	}
	if err := ValidateBranch(ctx, spec.Dir, spec.Branch); err != nil {
		return Result{}, err
	}
	if spec.Base != "" && (strings.HasPrefix(spec.Base, "-") || strings.ContainsAny(spec.Base, " \t\r\n")) {
		return Result{}, fmt.Errorf("base %q is not a valid branch or commit", spec.Base)
	}
	main, err := Root(ctx, spec.Dir)
	if err != nil {
		return Result{}, err
	}
	if _, err := presets.RunGit(ctx, main, nil, "rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
		return Result{}, errors.New("the repository has no commits yet; make a first commit before creating a worktree")
	}
	path := planned(ctx, main, spec.Name)
	if !linux(ctx) {
		if _, statErr := os.Stat(path); statErr == nil {
			return Result{}, fmt.Errorf("%s already exists; choose another worktree name", path)
		}
	}
	args := []string{"worktree", "add", "-b", spec.Branch, path}
	if spec.Base != "" {
		args = append(args, spec.Base)
	}
	if _, err := presets.RunGit(ctx, main, []string{"GIT_TERMINAL_PROMPT=0"}, args...); err != nil {
		return Result{}, err
	}
	result := Result{Path: path, MainRepo: main}
	if len(path) > MaxPathLength {
		result.Warnings = append(result.Warnings, fmt.Sprintf("the worktree path is %d characters long; deep folders may hit Windows path limits", len(path)))
	}
	copied, warnings := copyIncludes(ctx, main, path)
	result.Copied = copied
	result.Warnings = append(result.Warnings, warnings...)
	return result, nil
}

// Plan is what removing a worktree could lose.
type Plan struct {
	Dirty bool `json:"dirty"`
	// Unpushed counts commits reachable from this worktree that no remote has.
	Unpushed int    `json:"unpushed"`
	Branch   string `json:"branch"`
	Exists   bool   `json:"exists"`
}

// Preflight reports the state of a worktree before it is removed.
func Preflight(ctx context.Context, path string) Plan {
	status, ok := presets.GitStatus(ctx, path)
	if !ok {
		return Plan{}
	}
	plan := Plan{Dirty: status.Dirty, Branch: status.Branch, Exists: true}
	out, err := presets.RunGit(ctx, path, nil, "rev-list", "--count", "HEAD", "--not", "--remotes")
	if n, convErr := strconv.Atoi(strings.TrimSpace(out)); err == nil && convErr == nil {
		plan.Unpushed = n
	}
	return plan
}

// Remove deletes the linked worktree at path and, when deleteBranch is set, its
// branch. Uncommitted changes or unpushed commits are refused unless force is
// set. It never touches the main working tree.
func Remove(ctx context.Context, path, branch string, deleteBranch, force bool) error {
	info, ok := Inspect(ctx, path)
	if !ok || !sameDir(ctx, info.Path, path) {
		return fmt.Errorf("%s is not a linked git worktree; nothing was removed", path)
	}
	if deleteBranch && (branch == "" || branch == "detached") {
		return errors.New("there is no branch to delete")
	}
	plan := Preflight(ctx, path)
	if !force {
		if plan.Dirty {
			return errors.New("the worktree has uncommitted changes; commit or sync them, or force the removal")
		}
		if deleteBranch && plan.Unpushed > 0 {
			return fmt.Errorf("the branch has %s that no remote has; push them, or force the removal", plural(plan.Unpushed, "commit"))
		}
	}
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	if _, err := presets.RunGit(ctx, info.MainRepo, nil, append(args, path)...); err != nil {
		return err
	}
	_, _ = presets.RunGit(ctx, info.MainRepo, nil, "worktree", "prune")
	if !deleteBranch {
		return nil
	}
	flag := "-d"
	if force {
		flag = "-D"
	}
	if _, err := presets.RunGit(ctx, info.MainRepo, nil, "branch", flag, "--", branch); err != nil {
		return fmt.Errorf("the worktree was removed, but %w", err)
	}
	return nil
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// copyIncludes copies the ignored, untracked files that .worktreeinclude
// selects from the main working tree. Existing files are never overwritten.
func copyIncludes(ctx context.Context, main, target string) (int, []string) {
	if !hasIncludeFile(ctx, main) {
		return 0, nil
	}
	// Files matching the include patterns, then only those git ignores.
	listed, err := presets.RunGit(ctx, main, nil, "ls-files", "-z", "--others", "--ignored", "--exclude-from="+IncludeFile)
	if err != nil {
		return 0, []string{"could not read " + IncludeFile + ": " + err.Error()}
	}
	files := splitNUL(listed)
	if len(files) == 0 {
		return 0, nil
	}
	ignored, err := checkIgnored(ctx, main, files)
	if err != nil {
		return 0, []string{"could not check ignored files: " + err.Error()}
	}
	copied := 0
	var warnings []string
	for _, file := range ignored {
		if file == IncludeFile || file == ".git" || strings.HasPrefix(file, ".git/") {
			continue
		}
		ok, err := copyOne(ctx, main, target, file)
		switch {
		case err != nil:
			warnings = append(warnings, fmt.Sprintf("could not copy %s: %v", file, err))
		case ok:
			copied++
		}
	}
	return copied, warnings
}

func hasIncludeFile(ctx context.Context, main string) bool {
	if !linux(ctx) {
		_, err := os.Stat(filepath.Join(main, IncludeFile))
		return err == nil
	}
	out, err := presets.RunGit(ctx, main, nil, "ls-files", "--cached", "--others", "--", IncludeFile)
	return err == nil && strings.TrimSpace(out) != ""
}

func splitNUL(text string) []string {
	var files []string
	for _, item := range strings.Split(text, "\x00") {
		if item != "" {
			files = append(files, item)
		}
	}
	return files
}

// checkIgnored keeps the files git ignores. git check-ignore reads the names from stdin.
func checkIgnored(ctx context.Context, dir string, files []string) ([]string, error) {
	command, err := gitStdin(ctx, dir, strings.Join(files, "\x00")+"\x00", "check-ignore", "-z", "--stdin")
	if err != nil {
		return nil, err
	}
	out, runErr := command.Output()
	if runErr != nil {
		// Exit status 1 means none of the paths is ignored.
		var exit *exec.ExitError
		if errors.As(runErr, &exit) && exit.ExitCode() == 1 {
			return nil, nil
		}
		return nil, runErr
	}
	return splitNUL(string(out)), nil
}

// gitStdin builds a git command that reads input from stdin.
func gitStdin(ctx context.Context, dir, input string, args ...string) (*exec.Cmd, error) {
	var command *exec.Cmd
	if distro, inWSL := presets.WSLDistro(ctx); inWSL {
		if !strings.HasPrefix(dir, "/") {
			return nil, fmt.Errorf("%q is not a Linux path", dir)
		}
		command = exec.CommandContext(ctx, "wsl.exe", append([]string{"-d", distro, "--exec", "git", "-C", dir}, args...)...)
	} else {
		command = exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	}
	command.Stdin = strings.NewReader(input)
	presets.HideWindow(command)
	return command, nil
}

// copyOne copies one relative file when the target has none yet.
func copyOne(ctx context.Context, main, target, file string) (bool, error) {
	if strings.Contains(file, "..") {
		return false, errors.New("unsafe path")
	}
	if distro, inWSL := presets.WSLDistro(ctx); inWSL {
		source, destination := pathpkg.Join(main, file), pathpkg.Join(target, file)
		script := `[ -e "$2" ] && exit 3; [ -L "$1" ] && exit 4; mkdir -p "$(dirname "$2")" && cp -p -- "$1" "$2"`
		command := exec.CommandContext(ctx, "wsl.exe", "-d", distro, "--exec", "sh", "-c", script, "copy", source, destination)
		presets.HideWindow(command)
		err := command.Run()
		var exit *exec.ExitError
		if errors.As(err, &exit) && (exit.ExitCode() == 3 || exit.ExitCode() == 4) {
			return false, nil
		}
		return err == nil, err
	}
	source, destination := filepath.Join(main, filepath.FromSlash(file)), filepath.Join(target, filepath.FromSlash(file))
	info, err := os.Lstat(source)
	if err != nil || !info.Mode().IsRegular() {
		return false, nil
	}
	if _, err := os.Lstat(destination); err == nil {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return false, err
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(destination, data, info.Mode().Perm()); err != nil {
		return false, err
	}
	return true, nil
}
