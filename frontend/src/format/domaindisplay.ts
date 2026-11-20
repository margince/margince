// Company domains are stored in their punycode form so mail matching sees one
// spelling. People read the Unicode form, so display decodes it (RFC 3492).

const BASE = 36;
const T_MIN = 1;
const T_MAX = 26;
const SKEW = 38;
const DAMP = 700;
const INITIAL_BIAS = 72;
const INITIAL_N = 128;

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

function decodeLabel(encoded: string): string | undefined {
  const basicEnd = encoded.lastIndexOf("-");
  const output = Array.from(basicEnd > 0 ? encoded.slice(0, basicEnd) : "");
  let n = INITIAL_N;
  let bias = INITIAL_BIAS;
  let i = 0;
  let pos = basicEnd > 0 ? basicEnd + 1 : 0;
  while (pos < encoded.length) {
    const oldI = i;
    let w = 1;
    for (let k = BASE; ; k += BASE) {
      const digit = digitOf(encoded[pos++] ?? "");
      if (digit < 0) return undefined;
      i += digit * w;
      const t = k <= bias ? T_MIN : Math.min(Math.max(k - bias, T_MIN), T_MAX);
      if (digit < t) break;
      w *= BASE - t;
    }
    bias = adapt(i - oldI, output.length + 1, oldI === 0);
    n += Math.floor(i / (output.length + 1));
    i %= output.length + 1;
    output.splice(i++, 0, String.fromCodePoint(n));
  }
  return output.join("");
}

// A label that does not decode is shown as stored.
export function displayDomain(domain: string): string {
  return domain
    .split(".")
    .map((label) =>
      label.toLowerCase().startsWith("xn--")
        ? (decodeLabel(label.slice(4).toLowerCase()) ?? label)
        : label,
    )
    .join(".");
}
