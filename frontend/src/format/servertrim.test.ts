import { readFileSync } from "node:fs";
import { expect, it } from "vitest";
import { trimServerSpace, trimSubjectSpace } from "./servertrim";

// The eight characters `unicode.IsSpace` names outright for the Latin-1 range,
// which is the part of the mirror a reader can check against the Go source by
// eye. Both trims cut all of them.
const LATIN1_SPACE = ["\t", "\n", "\v", "\f", "\r", " ", "\u0085", "\u00a0"];

it.each(LATIN1_SPACE)("cuts %j from both ends, either set", (space) => {
  expect(trimServerSpace(`${space}${space}x${space}`)).toBe("x");
  expect(trimSubjectSpace(`${space}${space}x${space}`)).toBe("x");
});

// NEXT LINE is the character `String.prototype.trim` leaves standing, and a
// note holding only it once passed a form's "a note is present" check and was
// then refused by the server as missing.
it("treats a string of nothing but NEXT LINE as empty", () => {
  expect(trimServerSpace("\u0085\u0085")).toBe("");
  expect(trimSubjectSpace("\u0085")).toBe("");
});

// The one character the two sets disagree about, and the reason there are two
// of them: `strings.TrimSpace` keeps a byte-order mark, and `subjectSpace`
// widens itself to ECMAScript's set in order to cut it.
it("differs on the byte-order mark, and only on that", () => {
  expect(trimServerSpace("\uFEFFx\uFEFF")).toBe("\uFEFFx\uFEFF");
  expect(trimSubjectSpace("\uFEFFx\uFEFF")).toBe("x");
});

it("cuts the spaces that live above Latin-1", () => {
  expect(trimServerSpace("\u2028\u3000\u1680x")).toBe("x");
  expect(trimSubjectSpace("x\u2029\u2000")).toBe("x");
});

// Interior space is the value, not padding.
it("leaves the inside of the string alone", () => {
  expect(trimServerSpace("  a \u0085 b  ")).toBe("a \u0085 b");
});

// The shape the old regex went quadratic on: a long run of space that stops
// short of the end. The answer has to be right, whatever it costs to get.
it("handles a long run of space that does not reach either end", () => {
  const padded = `${" ".repeat(5000)}x${" ".repeat(5000)}y${" ".repeat(5000)}`;
  expect(trimServerSpace(padded)).toBe(`x${" ".repeat(5000)}y`);
});

it("hands back an empty string unchanged", () => {
  expect(trimServerSpace("")).toBe("");
  expect(trimSubjectSpace("")).toBe("");
});

// And the whole of it, against the server's own answers.
//
// Everything above is hand-written and checkable by eye, which is what makes it
// worth reading — and it is also a second opinion about what Go does. The
// corpus below is not: backend/gates/servertrimparity_test.go writes it from
// what `strings.TrimSpace` itself answers, and this reads that same file rather
// than a copy of it. A copy would be a third answer, and the drift this exists
// to catch is exactly the kind nobody notices.
//
// The Go side fails if the file is stale, and a sibling test there fails if the
// corpus stops reaching every character `unicode.IsSpace` names. So neither
// side can be corrected alone, and the corpus cannot quietly shrink.
type TrimCase = Readonly<{ in: string; want: string }>;

function codePointsOf(text: string): string {
  return (
    Array.from(text)
      .map(
        (char) =>
          `U+${(char.codePointAt(0) ?? 0).toString(16).toUpperCase().padStart(4, "0")}`,
      )
      .join(" ") || "(empty)"
  );
}

it("trims what the server trims, character for character", () => {
  const corpus: TrimCase[] = JSON.parse(
    readFileSync(
      new URL(
        "../../../backend/gates/testdata/servertrim.json",
        import.meta.url,
      ),
      "utf8",
    ),
  );
  // A corpus that failed to load reads exactly like a passing suite, so its
  // size is asserted before anything in it is.
  expect(corpus.length).toBeGreaterThan(100);
  // Code points, not characters: every character this exists for is invisible,
  // and a failure printing them shows two strings that look identical.
  const wrong = corpus
    .filter(({ in: input, want }) => trimServerSpace(input) !== want)
    .map(
      ({ in: input, want }) =>
        `[${codePointsOf(input)}] -> [${codePointsOf(trimServerSpace(input))}], the server says [${codePointsOf(want)}]`,
    );
  expect(wrong).toEqual([]);
});
