// What the server cuts from the ends of a string, so a form and the save it
// feeds agree on which values are empty.
//
// There are TWO answers here, not one, and that is the whole reason this module
// exists: they are a single character apart, and the two call sites had already
// drifted onto the wrong sides of it. A browser that trims a character the
// server keeps refuses a value the server would have taken; one that keeps a
// character the server trims sends a value the server then calls missing.
//
// JavaScript's own spellings are no help, which is why neither function below
// uses one. `String.prototype.trim` and `\s` cut ZWNBSP (U+FEFF) and leave NEXT
// LINE (U+0085) standing, and Go's `unicode.IsSpace` does exactly the opposite.
// The property both of them are approximating is Unicode's own White_Space, and
// `\p{White_Space}` IS `unicode.IsSpace` — so that is what is written out, and
// the one extra character is added only where the server adds it.

/** Unicode White_Space — rune for rune, Go's `unicode.IsSpace`. */
const SERVER_SPACE = /\p{White_Space}/u;

/** …and the byte-order mark on top, which is what ECMAScript's `\s` adds. */
const SUBJECT_SPACE = /[\p{White_Space}\uFEFF]/u;

// An index walk rather than `/^x+|x+$/`, which is quadratic: the trailing arm is
// unanchored at its start, so on a long run of spaces that does NOT reach the
// end of the string the engine retries the run from every position inside it.
//
// One UTF-16 unit at a time is safe because no White_Space character lives
// outside the BMP, so a surrogate half can never be one and `charAt` can never
// split one that is.
function trimEdges(value: string, isSpace: RegExp): string {
  let start = 0;
  let end = value.length;
  while (start < end && isSpace.test(value.charAt(start))) {
    start += 1;
  }
  while (end > start && isSpace.test(value.charAt(end - 1))) {
    end -= 1;
  }
  return value.slice(start, end);
}

/**
 * Trim the way `strings.TrimSpace` does — the server's trim, and so the one to
 * reach for wherever a form decides whether a field was filled in at all.
 */
export function trimServerSpace(value: string): string {
  return trimEdges(value, SERVER_SPACE);
}

/**
 * Trim the way `subjectSpace` does (activities/replysubject.go): `IsSpace` plus
 * the byte-order mark. That predicate widens itself to ECMAScript's set on
 * purpose, so a subject normalized in the browser and the same subject
 * normalized on the server come out as one string.
 */
export function trimSubjectSpace(value: string): string {
  return trimEdges(value, SUBJECT_SPACE);
}
