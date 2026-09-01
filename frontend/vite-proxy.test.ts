// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// Every origin-relative address the product HANDS SOMEBODY has to answer on the
// app's own port.
//
// The dev server is the app's origin, and it proxies a fixed list of prefixes;
// the api is a separate port a reader never sees. A connector screen that
// builds a copy-paste command from `location.origin` — which is right on a
// deployed stack, where the two are one host — therefore hands a developer a
// command that 404s while the endpoint answers correctly one port over. The
// reader goes looking for a misconfigured connector, a wrong secret, or a
// disabled extension. A broken example is worse than no example.
//
// So the prefixes are DERIVED from what the code builds rather than listed
// twice: a second connector publishing its own edge is covered the day it is
// written, instead of the day somebody remembers this file.

const here = dirname(fileURLToPath(import.meta.url));
const repo = join(here, "..");

// pathTemplate matches a template literal that OPENS with an interpolation —
// `` `${anything}/x/…` `` — and answers the segment after it, which is what a
// proxy entry is keyed on.
//
// Anchored on the backtick, so only the ROOT interpolation counts. Without it
// every later `${…}/segment` in the same literal matched too, and
// `${location.origin}/v1/offers/${offer.id}/pdf` was read as an address at
// /pdf — a prefix nothing serves, reported against a line that is correct.
const pathTemplate = /`\$\{[^}]*\}\/([a-z0-9._-]+)/g;

// namedOrigin narrows the same shape to a root interpolation that SAYS origin.
const namedOrigin = /`\$\{[^}]*\borigin\b[^}]*\}\/([a-z0-9._-]+)/gi;

// relativeImport answers the specifiers a module pulls from beside itself.
const relativeImport = /\bfrom\s+"(\.[^"]*)"/g;

function sourceFiles(dir: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir)) {
    if (entry === "node_modules" || entry === "dist" || entry.startsWith(".")) {
      continue;
    }
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) {
      out.push(...sourceFiles(path));
      continue;
    }
    if (/\.(ts|tsx)$/.test(entry) && !/\.test\.tsx?$/.test(entry)) {
      out.push(path);
    }
  }
  return out;
}

// proxiedPrefixes reads the proxy entries out of vite.config.ts.
//
// The BLOCK, matched by its braces, not everything from the marker to the end
// of the file. `indexOf` answers -1 when the marker moves or is renamed, and
// `slice(-1)` is then the file's last character — an empty set, reported only
// as the size floor further down, which reads as "the config has no proxy" when
// what happened is that this function stopped being able to find it. Scanning
// to EOF has the opposite failure too: any later object spelled `"key": {`
// counts as a proxied prefix and masks a genuinely missing route.
function proxiedPrefixes(): Set<string> {
  const config = readFileSync(join(here, "vite.config.ts"), "utf8");
  const marker = config.indexOf("proxy: {");
  expect(
    marker,
    "vite.config.ts has no `proxy: {` — this test derives the proxied set from that " +
      "block, and without it every assertion below would pass over an empty set",
  ).toBeGreaterThan(-1);

  let depth = 0;
  let end = marker;
  for (let i = config.indexOf("{", marker); i < config.length; i++) {
    if (config[i] === "{") depth++;
    if (config[i] === "}") depth--;
    if (depth === 0) {
      end = i;
      break;
    }
  }
  expect(
    end,
    "the `proxy: {` block in vite.config.ts is not brace-balanced",
  ).toBeGreaterThan(marker);

  const block = config.slice(marker, end);
  return new Set(
    [...block.matchAll(/"\/([a-z0-9._-]+)"\s*:\s*\{/gi)].map((m) => m[1]),
  );
}

// neighbourhood answers the files an address could be assembled across: the one
// reading `location.origin`, and the modules beside it that it imports.
//
// Because the READER of the origin and the BUILDER of the path are routinely
// two files. openchannel's screen takes `globalThis.location.origin` into a
// local and hands it to `inboundUrl(origin, endpoint)`; the template lives in
// contract.ts, one import away. A scan keyed on the word `origin` finds that
// one only because the parameter happens to be spelled so — rename it to `base`
// and /webhooks drops out of the census silently, which is the exact regression
// this file exists to catch.
function neighbourhood(file: string): string[] {
  const text = readFileSync(file, "utf8");
  const out = [file];
  for (const match of text.matchAll(relativeImport)) {
    const base = resolve(dirname(file), match[1]);
    for (const candidate of [
      `${base}.ts`,
      `${base}.tsx`,
      join(base, "index.ts"),
    ]) {
      try {
        if (statSync(candidate).isFile()) out.push(candidate);
      } catch {
        // A specifier this resolver cannot place is not a finding: it resolves
        // through tsconfig paths or a package, and neither assembles an address
        // beside the file that reads the origin.
      }
    }
  }
  return out;
}

describe("the dev server's proxy", () => {
  it("answers every origin-relative address the product hands somebody", () => {
    const proxied = proxiedPrefixes();
    // The floor: a scan that found nothing would report a clean pass over an
    // empty set.
    expect(proxied.size).toBeGreaterThan(4);

    const files = [
      ...sourceFiles(join(repo, "frontend", "src")),
      ...sourceFiles(join(repo, "extensions")),
    ];

    // TWO READINGS, unioned, because neither is sufficient alone.
    //
    // The first is the broad one: any template rooted at something SAYING
    // origin, wherever it sits. The second is the one that does not depend on
    // that word — every path template in the neighbourhood of a file that
    // actually reads `location.origin`. The second is why renaming a parameter
    // cannot quietly shrink this census.
    const built = new Map<string, string>();
    const byNeighbourhood = new Set<string>();
    for (const file of files) {
      const text = readFileSync(file, "utf8");
      for (const match of text.matchAll(namedOrigin)) {
        built.set(match[1], file.slice(repo.length + 1));
      }
      if (!/\blocation\.origin\b/.test(text)) continue;
      for (const near of neighbourhood(file)) {
        for (const match of readFileSync(near, "utf8").matchAll(pathTemplate)) {
          built.set(match[1], near.slice(repo.length + 1));
          byNeighbourhood.add(match[1]);
        }
      }
    }
    expect(built.size).toBeGreaterThan(0);
    // And the second reading found something of its own. Without this the union
    // above would go on passing after the neighbourhood walk broke, carried
    // entirely by the arm that reads the word.
    expect(
      byNeighbourhood.size,
      "no address was found through a file reading location.origin — the reading that does " +
        "not depend on an identifier being named `origin` has stopped reaching anything",
    ).toBeGreaterThan(0);

    for (const [prefix, file] of built) {
      expect(
        proxied.has(prefix),
        `${file} builds an address at /${prefix} from the app's own origin, which the dev ` +
          `server does not proxy — the command it hands a reader 404s on the app's port while ` +
          `the endpoint answers one port over. Add "/${prefix}" to the proxy list in vite.config.ts.`,
      ).toBe(true);
    }
  });
});
