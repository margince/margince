#!/usr/bin/env bash
# Prove check-comment-prose.sh fails each tell on an added comment line and
# passes what is not one: a string that holds the text, a directive, a waiver,
# a run of SQL keywords, a comment the change did not touch.
#
# Each case builds a throwaway repository with its own origin/main, so no case
# depends on the real tree's state or on another case.
set -uo pipefail
cd "$(dirname "$0")/.."

SRC="$(pwd)/scripts"
fails=0
ran=0

# case_ <name> <want-exit> <file> <content> — commit <content> as one change.
case_() {
	local name="$1" want="$2" file="$3" content="$4" dir got
	dir="$(mktemp -d)"
	mkdir -p "$dir/scripts" "$dir/$(dirname "$file")"
	cp "$SRC/check-comment-prose.sh" "$SRC/lib-commentscan.awk" "$SRC/lib-commentprose.awk" "$dir/scripts/"
	git -C "$dir" init -q --template=
	git -C "$dir" config user.email t@example.com
	git -C "$dir" config user.name t
	mkdir -p "$dir/backend" && printf 'package x\n\n// An old comment — left as it was.\nfunc Old() {}\n' > "$dir/backend/old.go"
	git -C "$dir" add -A && git -C "$dir" commit -q -m base
	git -C "$dir" branch -f origin/main HEAD
	git -C "$dir" symbolic-ref refs/remotes/origin/main refs/heads/origin/main
	printf '%s\n' "$content" > "$dir/$file"
	git -C "$dir" add -A && git -C "$dir" commit -q -m change
	(cd "$dir" && ./scripts/check-comment-prose.sh >/dev/null 2>&1)
	got=$?
	ran=$((ran + 1))
	if [[ "$got" -ne "$want" ]]; then
		echo "FAIL: $name: exit $got, want $want"
		fails=$((fails + 1))
	fi
	rm -rf "$dir"
}

case_ "an em dash in a new comment fails" 1 backend/a.go $'package x\n\n// The relay retries — then parks.\nfunc A() {}'
case_ "an over-used word fails" 1 backend/a.go $'package x\n\n// This is exactly one lock.\nfunc A() {}'
case_ "capitals for emphasis fail" 1 backend/a.go $'package x\n\n// It holds ONE lock per row.\nfunc A() {}'
case_ "a negative parallel fails" 1 backend/a.go $'package x\n\n// A retry is not a failure. It is the normal path.\nfunc A() {}'
case_ "change history fails" 1 backend/a.go $'package x\n\n// This change moves the lock.\nfunc A() {}'
case_ "a trailing comment is judged" 1 backend/a.go $'package x\n\nfunc A() {} // deliberately empty'
case_ "a TypeScript comment is judged" 1 frontend/src/a.ts $'// The rail holds ONE row.\nexport const a = 1;'
case_ "a clean comment passes" 0 backend/a.go $'package x\n\n// A retries the call twice, then parks it.\nfunc A() {}'
case_ "text inside a string passes" 0 backend/a.go $'package x\n\nconst s = "exactly — ONE"'
case_ "a directive passes" 0 backend/a.go $'//go:build !integration\n\npackage x'
case_ "a waiver passes" 0 backend/a.go $'package x\n\n// The card reads "Honest limits". prose:allow lexicon quoting the screen\nfunc A() {}'
case_ "a run of SQL keywords passes" 0 backend/a.go $'package x\n\n// The column is NOT NULL.\nfunc A() {}'
case_ "an untouched old comment passes" 0 backend/b.go $'package x\n\nfunc B() {}'

if (( fails > 0 )); then
	echo "test-comment-prose: $fails of $ran case(s) failed"
	exit 1
fi
echo "OK: test-comment-prose — $ran case(s), every tell caught and every exemption honoured"
