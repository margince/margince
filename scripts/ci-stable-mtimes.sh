#!/usr/bin/env bash
# ci-stable-mtimes.sh — give every tracked file, and every directory holding
# one, an mtime derived from its content, so a fresh checkout of unchanged
# content reads exactly as the last one did; and publish one digest per
# top-level entry, for the inputs Go's test cache never looks at.
#
# Go's test cache validates a replayed result against every file the test
# opened by size, mode and mtime — never content. A checkout stamps every file
# "now", so each package whose tests read the tree (a migration, a fixture, its
# own source) re-ran on every CI job although nothing it reads had changed.
#
# The mtime is a hash of the bytes on disk — never the index, which an unstaged
# edit leaves unchanged — folded into a date between 2000 and 2008: in the past,
# so Go's too-new cutoff never refuses it, and different for changed content
# unless 48 bits collide at the same size. A directory's is the hash of its
# tracked entries. Run it in the cache's writer and every reader alike, before
# the tests; a file written afterwards keeps its real time.
#
# The cache does not recheck a file outside the test's module at all, so under
# $GITHUB_ENV each top-level entry's digest is exported as TREE_DIGEST_<NAME>,
# and a test reading outside its module reads that variable instead
# (gatekit.DeclareInputs); the cache keys on every variable a test reads.
# --names prints each entry and its variable name, and touches nothing.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

export MODE="${1:-stamp}"
git ls-files -s -z | perl -0 -MDigest::SHA=sha1_hex -MTime::HiRes=utime -ne '
	chomp;
	my ($meta, $path) = split /\t/, $_, 2;
	my ($mode) = split / /, $meta;
	next if $mode eq "120000" || $mode eq "160000";
	my ($top) = split m{/}, $path, 2;
	if ($ENV{MODE} eq "--names") {
		$tops{$top} = 1;
		next;
	}
	my $content = Digest::SHA->new(1);
	$content->addfile("./$path", "b"); # "./" so a file named "-" is not read as stdin
	my $blob = $content->hexdigest;
	stamp($path, $blob);
	$tops{$top} .= "$path $blob\n";
	my $dir = $path;
	while ($dir =~ s{/[^/]+$}{}) {
		$entries{$dir} .= "$path $blob\n";
	}
	$entries{"."} .= "$path $blob\n";
	END {
		my %named;
		for my $top (sort keys %tops) {
			(my $name = "TREE_DIGEST_" . uc $top) =~ s/[^A-Z0-9_]/_/g;
			die "ci-stable-mtimes: $top and $named{$name} both map to $name\n" if $named{$name};
			$named{$name} = $top;
		}
		if ($ENV{MODE} eq "--names") {
			print "$named{$_}\t$_\n" for sort keys %named;
			exit 0;
		}
		stamp($_, sha1_hex($entries{$_})) for keys %entries;
		exit 0 unless $ENV{GITHUB_ENV};
		open my $env, ">>", $ENV{GITHUB_ENV} or die "ci-stable-mtimes: $ENV{GITHUB_ENV}: $!\n";
		print $env "$_=" . sha1_hex($tops{$named{$_}}) . "\n" for sort keys %named;
		close $env or die "ci-stable-mtimes: $ENV{GITHUB_ENV}: $!\n";
	}
	sub stamp {
		my ($path, $hash) = @_;
		my $bits = hex(substr($hash, 0, 12));
		my $time = 946684800 + ($bits % 2**28) + int($bits / 2**28) / 2**20;
		utime($time, $time, $path) or die "ci-stable-mtimes: $path: $!\n";
	}
'
