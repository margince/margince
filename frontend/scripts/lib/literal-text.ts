// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

const ESCAPE =
  /\\(?:u\{([\da-fA-F]+)\}|u([\da-fA-F]{4})|x([\da-fA-F]{2})|([0-3][0-7]{0,2}|[4-7][0-7]?)|(\r\n|[\n\r\u2028\u2029])|([\s\S]))/g;
const SIMPLE_ESCAPES: Record<string, string> = {
  n: "\n",
  r: "\r",
  t: "\t",
  b: "\b",
  f: "\f",
  v: "\v",
};

// cooked decodes every escape sequence in source, as the compiler cooks a string or template literal.
function cooked(source: string): string {
  return source.replace(
    ESCAPE,
    (
      sequence,
      braced?: string,
      unicode?: string,
      hex?: string,
      octal?: string,
      newline?: string,
      other?: string,
    ) => {
      const code = braced ?? unicode ?? hex;
      if (code !== undefined) {
        const point = Number.parseInt(code, 16);
        return point <= 0x10ffff ? String.fromCodePoint(point) : sequence;
      }
      if (octal !== undefined)
        return String.fromCharCode(Number.parseInt(octal, 8));
      if (newline !== undefined) return "";
      return other === undefined ? sequence : (SIMPLE_ESCAPES[other] ?? other);
    },
  );
}

const cookedFiles = new WeakMap<object, string>();

/** The file's text with every escape decoded, read once a run. */
export function cookedText(file: Readonly<{ text: string }>): string {
  const known = cookedFiles.get(file);
  if (known !== undefined) return known;
  const text = cooked(file.text);
  cookedFiles.set(file, text);
  return text;
}

// A token's text is a slice of its source, cooked or raw (a JSX attribute).
// So a miss in both rules out a parse, for a pattern not anchored to a token's ends.
export function mayHoldLiteral(
  source: string | Readonly<{ text: string }>,
  pattern: RegExp | string,
): boolean {
  const file = typeof source === "string" ? { text: source } : source;
  const holds = (text: string) =>
    typeof pattern === "string"
      ? text.includes(pattern)
      : text.search(pattern) !== -1;
  return (
    holds(file.text) || (file.text.includes("\\") && holds(cookedText(file)))
  );
}
