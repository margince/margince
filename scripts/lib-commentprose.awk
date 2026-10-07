# Reads one whole source file and reports each comment on an ADDED line (the
# `added` list) that breaks the prose bar in docs/reference/docs-prose-style.md.
# It runs after lib-commentscan.awk, so a `//` inside a string is never a comment,
# and it reads every line so string and block-comment state carries across them.
#
# Prints "<path>:<line>: [<rule>] <comment text>" per finding.

function blanks(n,   s) {
	s = ""
	while (n-- > 0) s = s " "
	return s
}

# commentText returns only the comment parts of a line: an inline `/* */` is
# cut at its terminator, so code after it is never judged as prose.
function commentText(line,   wasIn, c, at, rest, out) {
	wasIn = INBLOCK
	c = codeOf(line)
	gsub(/^[ \t]+|[ \t]+$/, "", c)
	if (c == "") return line
	out = ""
	if (wasIn && match(line, /\*\//)) {
		out = substr(line, 1, RSTART - 1)
		line = blanks(RSTART + 1) substr(line, RSTART + 2)
	}
	while ((at = commentAt(line)) > 0) {
		if (substr(line, at, 2) == "//") return out " " substr(line, at)
		rest = substr(line, at + 2)
		if (!match(rest, /\*\//)) return out " " substr(line, at)
		out = out " /*" substr(rest, 1, RSTART - 1)
		line = substr(line, 1, at - 1) blanks(RSTART + 3) substr(rest, RSTART + 2)
	}
	return out
}

function emphasisWord(w,   lw) {
	lw = tolower(w)
	sub(/'s$/, "", lw)
	return (lw in EMPHASIS)
}

function sqlWord(tok) {
	gsub(/[,.;:)(]/, "", tok)
	return (tok in SQL)
}

function report(rule, text) {
	if (rule == WAIVED) return
	printf "%s:%d: [%s] %s\n", path, FNR, rule, text
	found++
}

# waiver reads `prose:allow <rule> <reason>`, cuts it from the text and sets
# WAIVED; a waiver with no known rule or no reason is itself a finding.
function waiver(t, text,   at, rest, rule) {
	WAIVED = ""
	at = index(t, "prose:allow")
	if (at == 0) return t
	rest = substr(t, at + 11)
	sub(/^[ \t]+/, "", rest)
	rule = rest
	sub(/[ \t].*$/, "", rule)
	sub(/^[^ \t]*[ \t]*/, "", rest)
	if (rule in RULES && rest != "") WAIVED = rule
	else report("waiver", text)
	return substr(t, 1, at - 1)
}

function judge(text,   t, low, n, words, i, w) {
	t = text
	sub(/^[ \t]*(\/\/+|\/\*+|\*+\/?|\*)[ \t]?/, "", t)
	sub(/\*\/[ \t]*$/, "", t)
	if (t ~ /^(go:|gate:|craft:|nolint|lint:|#nosec|eslint|@ts-|biome-ignore|prettier-|rls-exempt|promptvoice:|promptlang:|SPDX-|Code generated)/) return
	t = waiver(t, text)
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
		# A run of SQL keywords, as in NOT NULL or ON CONFLICT DO NOTHING, is code.
		if (sqlWord(w) && ((i > 1 && sqlWord(words[i - 1])) || (i < n && sqlWord(words[i + 1])))) continue
		report("caps", text)
		break
	}
}

BEGIN {
	split("one not own only same both every never always all no any none before after first last this that the whole inside outside read write new old each must and or here there which from is are was twice once either neither nothing everything also still yet again more less most least really very then now until unless without with does do can cannot will may should", e, " ")
	for (k in e) EMPHASIS[e[k]] = 1
	split("ALL AND ANY AS ASC BY CASCADE CHECK CONFLICT CONSTRAINT CREATE DEFAULT DELETE DESC DISTINCT DO EACH EXISTS FIRST FOR FROM GROUP IN INSERT INTO IS KEY LAST LIMIT LOCKED NO NOT NOTHING NULL NULLS ON ONLY OR ORDER REFERENCES RETURNING ROW SELECT SET SHARE SKIP TABLE UNIQUE UPDATE USING VALUES WHERE WITH", q, " ")
	for (k in q) SQL[q[k]] = 1
	split("emdash lexicon residue negation caps", r, " ")
	for (k in r) RULES[r[k]] = 1
	n = split(added, a, ",")
	for (k = 1; k <= n; k++) if (a[k] != "") ADDED[a[k] + 0] = 1
}

{
	text = commentText($0)
	if ((FNR in ADDED) && text != "") judge(text)
}

END { exit found > 0 ? 1 : 0 }
