#!/usr/bin/env bash
# ci-stable-mtimes.sh — give every tracked file, and every directory holding
# one, an mtime derived from its content, so a fresh checkout of unchanged
# content reads exactly as the last one did.
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
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

git ls-files -s -z | perl -0 -MDigest::SHA=sha1_hex -MTime::HiRes=utime -ne '
	chomp;
	my ($meta, $path) = split /\t/, $_, 2;
	my ($mode) = split / /, $meta;
	next if $mode eq "120000" || $mode eq "160000";
	my $content = Digest::SHA->new(1);
	$content->addfile("./$path", "b"); # "./" so a file named "-" is not read as stdin
	my $blob = $content->hexdigest;
	stamp($path, $blob);
	my $dir = $path;
	while ($dir =~ s{/[^/]+$}{}) {
		$entries{$dir} .= "$path $blob\n";
	}
	$entries{"."} .= "$path $blob\n";
	END {
		stamp($_, sha1_hex($entries{$_})) for keys %entries;
	}
	sub stamp {
		my ($path, $hash) = @_;
		my $bits = hex(substr($hash, 0, 12));
		my $time = 946684800 + ($bits % 2**28) + int($bits / 2**28) / 2**20;
		utime($time, $time, $path) or die "ci-stable-mtimes: $path: $!\n";
	}
'
