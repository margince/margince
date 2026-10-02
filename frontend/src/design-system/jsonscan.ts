// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** What one pass over a JSON text found: where it stops parsing, and where each key sits. */
export type JsonScan = Readonly<{
  /** Offset of the first character that is not JSON; absent when it all is. */
  error?: number;
  /** Offset of each key by its dotted path (`provider.sort.by`, `provider.only[1]`). */
  keys: ReadonlyMap<string, number>;
}>;

const ESCAPES: Readonly<Record<string, string>> = {
  '"': '"',
  "\\": "\\",
  "/": "/",
  b: "\b",
  f: "\f",
  n: "\n",
  r: "\r",
  t: "\t",
};
const NUMBER = /-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?/y;
const HEX4 = /[0-9a-fA-F]{4}/y;

/**
 * Reads `text` as JSON the way the grammar does rather than the way an engine's
 * error message says, so the offset of a problem and the line of each key come
 * from one walk and do not depend on which browser is reading.
 */
export function scanJson(text: string): JsonScan {
  const scanner = new Scanner(text);
  const ok = scanner.document();
  return ok
    ? { keys: scanner.keys }
    : { error: scanner.at, keys: scanner.keys };
}

class Scanner {
  at = 0;
  readonly keys = new Map<string, number>();

  constructor(private readonly text: string) {}

  document(): boolean {
    this.space();
    if (!this.value("")) return false;
    this.space();
    return this.at === this.text.length;
  }

  private space() {
    while (" \t\n\r".includes(this.text[this.at] ?? "x")) this.at++;
  }

  private value(path: string): boolean {
    this.space();
    const c = this.text[this.at];
    if (c === "{") return this.object(path);
    if (c === "[") return this.array(path);
    if (c === '"') return this.string() !== null;
    for (const word of ["true", "false", "null"]) {
      if (this.text.startsWith(word, this.at)) {
        this.at += word.length;
        return true;
      }
    }
    return this.sticky(NUMBER);
  }

  private object(path: string): boolean {
    this.at++;
    this.space();
    if (this.text[this.at] === "}") {
      this.at++;
      return true;
    }
    for (;;) {
      if (!this.member(path)) return false;
      const after = this.separator("}");
      if (after !== "more") return after === "closed";
    }
  }

  /** One `"key": value` of the object at path; a repeated key keeps its first line. */
  private member(path: string): boolean {
    this.space();
    const keyAt = this.at;
    const key = this.text[this.at] === '"' ? this.string() : null;
    if (key === null) return false;
    this.space();
    if (this.text[this.at] !== ":") return false;
    this.at++;
    const child = path ? `${path}.${key}` : key;
    if (!this.keys.has(child)) this.keys.set(child, keyAt);
    return this.value(child);
  }

  private array(path: string): boolean {
    this.at++;
    this.space();
    if (this.text[this.at] === "]") {
      this.at++;
      return true;
    }
    for (let index = 0; ; index++) {
      this.space();
      this.keys.set(`${path}[${index}]`, this.at);
      if (!this.value(`${path}[${index}]`)) return false;
      const after = this.separator("]");
      if (after !== "more") return after === "closed";
    }
  }

  /** Past a comma (more to read), past the closer (closed), or neither. */
  private separator(closer: string): "more" | "closed" | "bad" {
    this.space();
    const c = this.text[this.at];
    if (c !== "," && c !== closer) return "bad";
    this.at++;
    return c === "," ? "more" : "closed";
  }

  /** The string at the cursor, decoded; null where it stops being one. */
  private string(): string | null {
    this.at++;
    let out = "";
    while (this.at < this.text.length) {
      const c = this.text[this.at];
      if (c === '"') {
        this.at++;
        return out;
      }
      if (c < " ") return null;
      if (c === "\\") {
        const escaped = this.escape();
        if (escaped === null) return null;
        out += escaped;
        continue;
      }
      out += c;
      this.at++;
    }
    return null;
  }

  private escape(): string | null {
    const kind = this.text[this.at + 1] ?? "";
    const plain = Object.hasOwn(ESCAPES, kind) ? ESCAPES[kind] : undefined;
    if (plain !== undefined) {
      this.at += 2;
      return plain;
    }
    if (kind !== "u") return null;
    this.at += 2;
    const start = this.at;
    if (!this.sticky(HEX4)) return null;
    return String.fromCharCode(
      Number.parseInt(this.text.slice(start, this.at), 16),
    );
  }

  private sticky(pattern: RegExp): boolean {
    pattern.lastIndex = this.at;
    const found = pattern.exec(this.text);
    if (!found) return false;
    this.at += found[0].length;
    return true;
  }
}
