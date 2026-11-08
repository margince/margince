#!/usr/bin/env bash
# keep-unchanged-mtimes.sh save|restore <snapshot> — around a generator: `save`
# records the tracked files, `restore` gives back the old mtime of every one the
# generator rewrote with the bytes it already held. Two steps rather than a
# wrapper, so a recipe keeps its `$(MAKE)` line and `make -n` still dry-runs it.
#
# Go replays a cached test result only while every file the test opened keeps
# its size, mode and mtime. The generators behind `make check` write their
# committed outputs on every run, so the packages whose tests read them ran
# again on a tree that had not changed. ci-stable-mtimes.sh answers the same
# question in CI by stamping the whole tree; on a working checkout that would
# also back-date the files a developer just edited.
#
# Only a file that matched the index before the generator and matches it after
# keeps its old time: then its bytes are provably the same. A file that was
# already edited, or that the generator changed, keeps the time it was written.
set -euo pipefail

usage() {
	echo "usage: keep-unchanged-mtimes.sh save|restore <snapshot>" >&2
	exit 2
}

# save writes path, index blob and mtime of every clean tracked file. `git diff`
# refreshes the index first, so a file whose time moved with no change in
# content counts as clean.
save() {
	git -C "$top" ls-files -s -z | perl -0 -MTime::HiRes=stat -e '
		open my $diff, "-|", "git", "-C", $ARGV[0], "diff", "--name-only", "-z"
			or die "keep-unchanged-mtimes: cannot run git diff: $!\n";
		my %dirty = map { $_ => 1 } split /\0/, do { local $/; <$diff> // "" };
		# A git diff that failed lists no dirty file, and every edit would count as clean.
		close $diff or die "keep-unchanged-mtimes: git diff failed\n";
		chdir $ARGV[0] or die "keep-unchanged-mtimes: cannot enter $ARGV[0]: $!\n";
		while (my $row = <STDIN>) {
			chomp $row;
			my ($meta, $path) = split /\t/, $row, 2;
			my ($mode, $blob) = split / /, $meta;
			# --stdin-paths reads one path per line and C-unquotes one that opens with a
			# quote, so such a path is left as written.
			next if $mode eq "120000" || $mode eq "160000" || $dirty{$path} || $path =~ /\n|^"/;
			my @st = stat $path or next;
			# One fixed spelling, compared as text: a float printed plainly keeps 15
			# digits, fewer than a nanosecond mtime has, and every file would look moved.
			printf "%s\0%s\0%.9f\0", $path, $blob, $st[9];
		}
	' "$top" >"$snapshot"
}

# restore hashes only the files whose mtime moved, and puts the old one back on
# each whose bytes still match the index blob it had.
restore() {
	[[ -f "$snapshot" ]] || {
		echo "keep-unchanged-mtimes: no snapshot at $snapshot — run save first" >&2
		exit 2
	}
	local moved="$snapshot.moved" hashes="$snapshot.hashes"
	perl -0 -MTime::HiRes=stat -e '
		chdir $ARGV[0] or die "keep-unchanged-mtimes: cannot enter $ARGV[0]: $!\n";
		open my $in, "<", $ARGV[1] or die "keep-unchanged-mtimes: $!\n";
		while (defined(my $path = <$in>)) {
			chomp $path;
			chomp(my $blob = <$in>);
			chomp(my $mtime = <$in>);
			my @st = stat $path or next;
			next if sprintf("%.9f", $st[9]) eq $mtime;
			printf "%s\n%s\n%s\n%.9f\n", $path, $blob, $mtime, $st[8];
		}
	' "$top" "$snapshot" >"$moved"
	# The paths go to git from a file, not a pipe it answers on, so a long list
	# cannot fill both pipes and stall the two processes on each other.
	awk 'NR % 4 == 1' "$moved" | (cd "$top" && git hash-object --stdin-paths) >"$hashes"
	perl -MTime::HiRes=utime -e '
		chdir $ARGV[0] or die "keep-unchanged-mtimes: cannot enter $ARGV[0]: $!\n";
		open my $moved, "<", $ARGV[1] or die "keep-unchanged-mtimes: $!\n";
		open my $hashes, "<", $ARGV[2] or die "keep-unchanged-mtimes: $!\n";
		while (defined(my $path = <$moved>)) {
			chomp $path;
			chomp(my $blob = <$moved>);
			chomp(my $mtime = <$moved>);
			chomp(my $atime = <$moved>);
			my $now = <$hashes> // die "keep-unchanged-mtimes: git hashed fewer files than moved\n";
			chomp $now;
			next unless $now eq $blob;
			utime $atime, $mtime, $path or die "keep-unchanged-mtimes: $path: $!\n";
		}
	' "$top" "$moved" "$hashes"
	rm -f "$snapshot" "$moved" "$hashes"
}

(($# == 2)) || usage
mode="$1"
# Absolute, because both steps read the tree from its top directory.
mkdir -p "$(dirname "$2")"
snapshot="$(cd "$(dirname "$2")" && pwd)/$(basename "$2")"
top="$(git rev-parse --show-toplevel)"
case "$mode" in
save) save ;;
restore) restore ;;
*) usage ;;
esac
