// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H3

package gates

// What belongs to the REPOSITORY, as against what happens to be in a working
// tree.
//
// A gate that walks the filesystem judges whatever a checkout carries: a build
// cache a package manager wrote, a page somebody is drafting, a scratch file
// from a parallel session. Those are not the repository. Failing on them makes
// `make check-backend` untrustworthy locally, and the habit of skipping past a
// failure is the expensive part — the next one looks the same and is real.
//
// So the walks ask git. Ignored paths are skipped entirely; where a census must
// judge only what is committed, it reads the tracked set. Both answers come
// from one place so the two cannot drift, and both FAIL LOUDLY when git cannot
// answer rather than judging a smaller tree and reporting the same word for it.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// gitIgnoredPaths answers the absolute paths git ignores under root — build
// caches, node_modules, a .pnpm-store somebody's install left at the top.
//
// Directories, so a walk can skip a whole subtree at its head rather than
// asking about every file under it.
func gitIgnoredPaths(t testing.TB, root string) map[string]bool {
	t.Helper()
	out := gitPaths(t, root, "ls-files", "--others", "--ignored", "--directory", "--exclude-standard")
	ignored := map[string]bool{}
	for _, line := range out {
		// Compared against filepath.WalkDir paths, which are the host's own
		// spelling — so back from slashes here rather than on to them.
		ignored[filepath.Join(root, filepath.FromSlash(line))] = true
	}
	return ignored
}

// gitTracked answers the paths git tracks under root, relative to it. A file
// that is not in here is not part of the repository yet, whatever a working
// tree holds.
func gitTracked(t testing.TB, root string) map[string]bool {
	t.Helper()
	tracked := map[string]bool{}
	for _, line := range gitPaths(t, root, "ls-files") {
		// SLASHES, because that is what every caller keys on: a page is named
		// by filepath.ToSlash(rel) so one name reads the same in a message
		// whatever host printed it. filepath.Clean alone would key this side in
		// backslashes on Windows, the two would never meet, and the census
		// would drop every page — which its own emptiness guard would then
		// report as a repository with no documentation in it.
		tracked[path.Clean(line)] = true
	}
	// A repository with no tracked files is not a repository this gate can
	// judge, and reading zero is how a census passes over nothing.
	if len(tracked) == 0 {
		t.Fatalf("git tracks no file under %s — this census would judge an empty tree "+
			"and report the same word for it as a full one", root)
	}
	return tracked
}

// gitPaths runs one git query and answers the paths it names, verbatim.
//
// `-z` rather than lines, and NO trimming, because a path is not a word. Git
// quotes any name carrying a non-ASCII byte by default (core.quotepath), and
// a line-oriented read then keys `"caf\303\251.md"` — a string no walk will
// ever produce. Trimming loses the other end of the same problem: a file whose
// name begins or ends with a space is legal on every filesystem this runs on,
// and its key would name a different file. Both make the census judge less than
// it thinks while reporting the same word for it. Under `-z` git emits the raw
// bytes and separates them with NUL, which no path may contain.
//
// A git that cannot answer is fatal: every caller is a census, and a census
// that quietly judges less than it thinks is the failure each of them exists to
// prevent.
func gitPaths(t testing.TB, root string, args ...string) []string {
	t.Helper()
	cmd := exec.Command("git", append(args, "-z")...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		// git's own diagnostic, not just the status. cmd.Output() puts it in
		// ExitError.Stderr and Error() renders "exit status 128" — which tells
		// a reader that something failed and nothing about what, on a gate
		// whose whole posture is to fail loudly. "fatal: not a git repository"
		// is the sentence they need.
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(exit.Stderr) > 0 {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exit.Stderr)))
		}
		t.Fatalf("git %s in %s: %v — this gate reads the repository through git, and "+
			"cannot tell a clean tree from an unreadable one", strings.Join(args, " "), root, err)
	}
	var paths []string
	for _, entry := range strings.Split(string(out), "\x00") {
		if entry != "" {
			paths = append(paths, entry)
		}
	}
	return paths
}

// The two readings, driven over a repository built for the purpose.
//
// Built rather than borrowed: whether THIS checkout happens to carry a build
// cache is a fact about the machine, and a case that passes because somebody
// ran pnpm install proves nothing on a clean runner. A repository with a known
// ignore rule and a known tracked file asks the question directly.
func TestTheRepositoryIsReadThroughGitRatherThanTheWorkingTree(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	run, write := initRepo(t, root, nil)

	write(".gitignore", "/cache/\n")
	write("kept.md", "# kept\n")
	// A NESTED page, and one whose name git would quote under a line-oriented
	// read: `core.quotepath` escapes the non-ASCII bytes, and the key would
	// then name a file no walk produces. Nested because the separator is the
	// other half of the same problem — a key spelled with backslashes on one
	// host and slashes on the other never meets its walk.
	write("docs/reference/kept.md", "# nested\n")
	write("docs/reference/caf\u00e9.md", "# accented\n")
	// The shape the reports were about: a build cache holding a symlink, which
	// a filesystem walk meets and refuses.
	write("cache/store/marker", "")
	if err := os.Symlink(root, filepath.Join(root, "cache", "store", "link")); err != nil {
		t.Fatalf("planting the symlink a build cache leaves: %v", err)
	}
	// And a page somebody is drafting: present, not ignored, not committed.
	write("draft.md", "# draft\n")
	run("add", ".gitignore", "kept.md", "docs")
	run("commit", "-qm", "first")

	tracked := gitTracked(t, root)
	// Keyed the way every caller names a page: slash-separated, and spelled in
	// the bytes the filesystem holds. A nested path is where a host's own
	// separator would show, and an accented one is where git's default quoting
	// would — either way the key names a file no walk produces, and the census
	// drops it without a word.
	for _, page := range []string{"kept.md", "docs/reference/kept.md", "docs/reference/café.md"} {
		if !tracked[page] {
			t.Errorf("%q is committed and is not reported as tracked, so the census would judge a "+
				"smaller tree than the repository holds: %v", page, tracked)
		}
	}
	if tracked["draft.md"] {
		t.Error("a file nobody has committed is reported as tracked — a page being drafted would " +
			"fail a budget it is not yet subject to")
	}
	if tracked["cache/store/marker"] {
		t.Error("a file under an ignored directory is reported as tracked")
	}

	ignored := gitIgnoredPaths(t, root)
	if !ignored[filepath.Join(root, "cache")] {
		t.Errorf("the ignored directory is not reported, so a walk would descend into it and "+
			"meet the symlink inside: %v", ignored)
	}
}

// gitFixture answers the two things a purpose-built repository is made of: a
// way to run git in it, and a way to put a file in it.
//
// Shared rather than copied per test, because two copies of a fixture are two
// fixtures — and the day one of them stops failing loudly on a git error is the
// day its test starts passing over a repository that was never built.
// initRepo is gitFixture with the bootstrap done: an initialised repository with
// an identity to commit under, and whatever first files the caller names already
// committed.
//
// Shared because three tests were each spelling out the same five git calls, and
// a bootstrap that drifts between copies is three fixtures pretending to be one.
func initRepo(t *testing.T, root string, committed map[string]string) (run func(...string), write func(rel, body string)) {
	t.Helper()
	run, write = gitFixture(t, root)
	run("init", "-q")
	run("config", "user.email", "gate@example.test")
	run("config", "user.name", "Gate")
	if len(committed) == 0 {
		return run, write
	}
	paths := make([]string, 0, len(committed))
	for rel, body := range committed {
		write(rel, body)
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	run(append([]string{"add"}, paths...)...)
	run("commit", "-qm", "first")
	return run, write
}

func gitFixture(t *testing.T, root string) (run func(...string), write func(rel, body string)) {
	t.Helper()
	run = func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	write = func(rel, body string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return run, write
}

// THE FAIL-LOUDLY BRANCHES, driven.
//
// This file's whole posture is that a git it cannot read is fatal — a census
// that quietly judges less than it thinks is the failure both its callers exist
// to prevent. Nothing exercised those branches: a regression making the queries
// succeed with empty output would have gone green, which is precisely the shape
// being guarded against.
//
// Driven through a recorder rather than a *testing.T, because a Fatalf on the
// real one ends the test that is trying to observe it.
func TestAGitThatCannotAnswerIsFatalRatherThanAnEmptyTree(t *testing.T) {
	t.Parallel()
	// A directory that is no repository. git exits non-zero and says so.
	outside := t.TempDir()
	t.Run("a query git refuses", func(t *testing.T) {
		t.Parallel()
		rec := &fatalRecorder{}
		rec.run(func() { gitPaths(rec, outside, "ls-files") })
		if !rec.fatal {
			t.Error("git refused the query and the reading carried on — a census cannot tell a clean " +
				"tree from an unreadable one, which is the whole of what this file is for")
		}
		if !strings.Contains(rec.message, "not a git repository") {
			t.Errorf("the failure reads %q, which does not carry git's own diagnostic", rec.message)
		}
	})
	t.Run("a repository tracking nothing", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		run, _ := initRepo(t, root, nil)
		_ = run
		rec := &fatalRecorder{}
		rec.run(func() { gitTracked(rec, root) })
		if !rec.fatal {
			t.Error("an empty tracked set was returned rather than refused — every caller would then " +
				"judge nothing and report the same word for it as a full tree")
		}
	})
}

// fatalRecorder is a testing.TB that records a Fatalf instead of ending the
// test observing it. Only the methods these helpers call do anything; the rest
// satisfy the interface.
type fatalRecorder struct {
	testing.TB
	fatal   bool
	message string
	done    chan struct{}
}

func (r *fatalRecorder) Helper() {}

func (r *fatalRecorder) Fatalf(format string, args ...any) {
	r.fatal = true
	r.message = fmt.Sprintf(format, args...)
	// The same control flow a real Fatalf has: the caller must not carry on
	// past it, or this records a failure the helper never actually stopped for.
	close(r.done)
	runtime.Goexit()
}

// run calls fn on a goroutine of its own, so a Goexit from Fatalf ends that
// goroutine rather than the test.
func (r *fatalRecorder) run(fn func()) {
	r.done = make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		fn()
	}()
	<-finished
}
