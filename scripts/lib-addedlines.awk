# Reads a `git diff --unified=0` of one file and prints the post-image line
# numbers it adds, comma-separated.
/^@@/ {
	match($0, /\+[0-9]+(,[0-9]+)?/)
	n = split(substr($0, RSTART + 1, RLENGTH - 1), span, ",")
	count = n > 1 ? span[2] : 1
	for (i = 0; i < count; i++) printf "%d,", span[1] + i
}
