// Which modules a source file pulls in, read the way the module system reads
// them. fe-uat.mjs maps each story to the files it renders with this.

// Coverage means a story RENDERS the component, so a specifier only counts
// where the module system would honour it: comments are stripped first, and
// what remains has to sit in a real import/export position — a path mentioned
// in prose or quoted as data pulls nothing in.
const STATEMENT_START = String.raw`(?:^|[;{}])\s*`;
const IMPORT_PATTERNS = [
  // `import … from "s"` / `export … from "s"`. The clause between the keyword
  // and `from` carries neither a quote nor a `;`, which is what stops a match
  // from spanning two statements while still crossing a multi-line clause.
  new RegExp(
    `${STATEMENT_START}(?:import|export)\\b[^;'"]*?\\bfrom\\s*["']([^"']+)["']`,
    "gm",
  ),
  // Side-effect `import "s"`.
  new RegExp(`${STATEMENT_START}import\\s+["']([^"']+)["']`, "gm"),
  // Dynamic `import("s")`, which is an expression and has no statement start.
  /\bimport\s*\(\s*["']([^"']+)["']/g,
];

// A comment or a string literal, whichever opens first. String literals are in
// the alternation so a `//` inside one — a URL, say — is consumed as part of
// the string and never read as the start of a comment.
const COMMENT_OR_STRING =
  /\/\/[^\n]*|\/\*[\s\S]*?\*\/|"(?:[^"\\\n]|\\.)*"|'(?:[^'\\\n]|\\.)*'|`(?:[^`\\]|\\.)*`/g;

// Blanks out comments, leaving string literals alone. Blanking rather than
// deleting keeps every line and column where it was, so the statement-start
// anchors above still see real line starts.
function stripComments(source) {
  return source.replace(COMMENT_OR_STRING, (match) =>
    match.startsWith("/") ? match.replace(/[^\n]/g, " ") : match,
  );
}

// The module specifiers of `import x from "s"`, `import "s"`, `export … from "s"`
// and `import("s")` — every form by which a story pulls in a sibling module.
export function importSpecifiers(source) {
  const code = stripComments(source);
  const specifiers = [];
  for (const pattern of IMPORT_PATTERNS) {
    for (const match of code.matchAll(pattern)) specifiers.push(match[1]);
  }
  return specifiers;
}
