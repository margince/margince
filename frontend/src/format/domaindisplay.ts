// Company domains are stored in their punycode form so mail matching sees one
// spelling. Readers expect the Unicode form, so display decodes it (RFC 3492)
// when that is safe to show, and keeps the stored form otherwise.

const BASE = 36;
const T_MIN = 1;
const T_MAX = 26;
const SKEW = 38;
const DAMP = 700;
const INITIAL_BIAS = 72;
const INITIAL_N = 128;
const MAX_CODE_POINT = 0x10ffff;

function adapt(delta: number, points: number, first: boolean): number {
  let d = first ? Math.floor(delta / DAMP) : delta >> 1;
  d += Math.floor(d / points);
  let k = 0;
  while (d > ((BASE - T_MIN) * T_MAX) >> 1) {
    d = Math.floor(d / (BASE - T_MIN));
    k += BASE;
  }
  return k + Math.floor(((BASE - T_MIN + 1) * d) / (d + SKEW));
}

function digitOf(char: string): number {
  const code = char.charCodeAt(0);
  if (code >= 48 && code <= 57) return code - 22;
  if (code >= 97 && code <= 122) return code - 97;
  return -1;
}

// Reads one variable-length integer; undefined on a bad digit or overflow.
function readDelta(
  encoded: string,
  start: number,
  i: number,
  bias: number,
): { i: number; pos: number } | undefined {
  let pos = start;
  let next = i;
  let w = 1;
  for (let k = BASE; ; k += BASE) {
    const digit = digitOf(encoded[pos++] ?? "");
    if (digit < 0) return undefined;
    next += digit * w;
    const t = k <= bias ? T_MIN : Math.min(Math.max(k - bias, T_MIN), T_MAX);
    if (digit < t) break;
    w *= BASE - t;
    if (!Number.isSafeInteger(next) || !Number.isSafeInteger(w)) {
      return undefined;
    }
  }
  return Number.isSafeInteger(next) ? { i: next, pos } : undefined;
}

function decodeLabel(encoded: string): string | undefined {
  const basicEnd = encoded.lastIndexOf("-");
  const output = Array.from(basicEnd > 0 ? encoded.slice(0, basicEnd) : "");
  let n = INITIAL_N;
  let bias = INITIAL_BIAS;
  let i = 0;
  let pos = basicEnd > 0 ? basicEnd + 1 : 0;
  while (pos < encoded.length) {
    const oldI = i;
    const read = readDelta(encoded, pos, i, bias);
    if (!read) return undefined;
    ({ i, pos } = read);
    bias = adapt(i - oldI, output.length + 1, oldI === 0);
    n += Math.floor(i / (output.length + 1));
    if (n > MAX_CODE_POINT) return undefined;
    i %= output.length + 1;
    output.splice(i++, 0, String.fromCodePoint(n));
  }
  return output.join("");
}

const PLAIN = /^[\p{L}\p{M}\p{N}-]+$/u;
const SCRIPTS = [
  /\p{Script=Latin}/u,
  /\p{Script=Cyrillic}/u,
  /\p{Script=Greek}/u,
  /\p{Script=Arabic}/u,
  /\p{Script=Hebrew}/u,
  /\p{Script=Devanagari}/u,
  /\p{Script=Thai}/u,
  /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}\p{Script=Hangul}\p{Script=Bopomofo}]/u,
];

// One script per label; a letter outside the listed scripts counts as its own.
function singleScript(label: string): boolean {
  const seen = new Set<number>();
  for (const char of label) {
    if (!/\p{L}/u.test(char)) continue;
    const index = SCRIPTS.findIndex((script) => script.test(char));
    seen.add(index);
  }
  return seen.size <= 1 && !seen.has(-1);
}

function decodedLabel(label: string): string | undefined {
  if (!label.toLowerCase().startsWith("xn--")) return label;
  const decoded = decodeLabel(label.slice(4).toLowerCase());
  if (decoded === undefined || !PLAIN.test(decoded) || !singleScript(decoded)) {
    return undefined;
  }
  return decoded;
}

// The Unicode form when every label decodes safely, else the stored form.
export function displayDomain(domain: string): string {
  const labels = domain.split(".").map(decodedLabel);
  return labels.every((label) => label !== undefined)
    ? labels.join(".")
    : domain;
}
