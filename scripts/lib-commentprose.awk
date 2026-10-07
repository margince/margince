# Reads a unified diff and reports each ADDED comment line that breaks the prose
# bar in docs/reference/docs-prose-style.md: the tells a reader skims past. It
# runs after lib-commentscan.awk, so a `//` inside a string is never a comment.
#
# Prints "<path>:<line>: [<rule>] <comment text>" per finding.

function commentText(line,   c, at) {
	c = codeOf(line)
	gsub(/^[ \t]+|[ \t]+$/, "", c)
	if (c == "") return line
	at = commentAt(line)
	return at > 0 ? substr(line, at) : ""
}

function emphasisWord(w,   lw) {
	lw = tolower(w)
	sub(/'s$/, "", lw)
	return (lw in EMPHASIS)
}

function capsToken(tok) {
	gsub(/[,.;:)(]/, "", tok)
	return length(tok) > 1 && tok == toupper(tok) && tok != tolower(tok)
}

function report(rule, text) {
	printf "%s:%d: [%s] %s\n", path, lineno, rule, text
	found++
}

function judge(text,   t, low, n, words, i, w, prev, next_) {
	t = text
	sub(/^[ \t]*(\/\/+|\/\*+|\*+\/?|\*)[ \t]?/, "", t)
	sub(/\*\/[ \t]*$/, "", t)
	if (t ~ /^(go:|gate:|craft:|nolint|lint:|#nosec|eslint|@ts-|biome-ignore|prettier-|rls-exempt|promptvoice:|promptlang:|SPDX-|Code generated)/) return
	if (t ~ /prose:allow/) return
	gsub(/`[^`]*`/, " ", t)
	low = tolower(t)
	if (index(t, "—") > 0) report("emdash", text)
	if (match(low, /(^|[^a-z])(honest|honestly|honesty|genuine|genuinely|deliberate|deliberately|quietly|load-bearing|exactly)([^a-z]|$)/) ||
		index(low, "on purpose") || index(low, "is the point") || index(low, "the whole point") || index(low, "earns its place"))
		report("lexicon", text)
	if (match(low, /(^|[^a-z])(this pr|this change|first cut|at the time of writing)([^a-z]|$)/)) report("residue", text)
	if (match(t, /(is|are|was|were) not [^.]+\. +(It|They|That|This) (is|are|was|were)[^a-z]/)) report("negation", text)
	n = split(t, words, /[ \t]+/)
	for (i = 1; i <= n; i++) {
		w = words[i]; gsub(/[,.;:!?)("]/, "", w)
		if (w !~ /^[A-Z][A-Z']+$/ || !emphasisWord(w)) continue
		prev = (i > 1) ? words[i - 1] : ""; next_ = (i < n) ? words[i + 1] : ""
		if (capsToken(prev) || capsToken(next_)) continue
		report("caps", text)
		break
	}
}

BEGIN {
	split("one not own only same both every never always all no any none before after first last this that the whole inside outside read write new old each must and or here there which from is are was twice once either neither nothing everything also still yet again more less most least really very then now until unless without with does do can cannot will may should", e, " ")
	for (k in e) EMPHASIS[e[k]] = 1
}

/^--- / { next }

/^\+\+\+ / {
	path = substr($0, 7)
	want = (path ~ /\.(go|ts|tsx)$/) && (path !~ /(_gen\.go|\.gen\.go|\.d\.ts)$/) && (path !~ /\/testdata\//)
	INBLOCK = 0; STACK = ""; STACKIN = ""
	next
}

/^@@/ {
	INBLOCK = 0; STACK = ""; STACKIN = ""
	match($0, /\+[0-9]+/)
	lineno = substr($0, RSTART + 1, RLENGTH - 1) - 1
	next
}

!want { next }

/^\+/ {
	lineno++
	line = substr($0, 2)
	text = commentText(line)
	if (text != "") judge(text)
	next
}

/^ / { lineno++; next }

END { exit found > 0 ? 1 : 0 }
