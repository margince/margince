// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// What nginx.conf answers for a path, worked out from the file the image ships
// by nginx's own location rules: an exact match first, then the longest
// prefix, which ends the search when it is ^~, then the regexes in file order,
// then that longest prefix.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const conf = readFileSync(join(frontendRoot, "nginx.conf"), "utf8")
  .split("\n")
  .map((line) => line.replace(/^\s*#.*$/, ""))
  .join("\n");

type Location = { modifier: string; pattern: string; body: string };

// Each location's own body, up to the brace that closes it.
function locations(): Location[] {
  const found: Location[] = [];
  const head = /^\s*location\s+(=|\^~|~\*?|)\s*("[^"]*"|[^\s{]+)\s*\{/gm;
  for (const match of conf.matchAll(head)) {
    let depth = 1;
    let end = (match.index ?? 0) + match[0].length;
    while (depth > 0 && end < conf.length) {
      if (conf[end] === "{") depth++;
      if (conf[end] === "}") depth--;
      end++;
    }
    found.push({
      modifier: match[1],
      pattern: match[2].replace(/^"|"$/g, ""),
      body: conf.slice((match.index ?? 0) + match[0].length, end - 1),
    });
  }
  return found;
}

function locate(path: string): Location {
  const all = locations();
  const exact = all.find((l) => l.modifier === "=" && l.pattern === path);
  if (exact) return exact;
  const prefix = all
    .filter(
      (l) =>
        (l.modifier === "" || l.modifier === "^~") &&
        path.startsWith(l.pattern),
    )
    .sort((a, b) => b.pattern.length - a.pattern.length)[0];
  if (prefix?.modifier === "^~") return prefix;
  const regex = all.find(
    (l) => l.modifier === "~" && new RegExp(l.pattern).test(path),
  );
  const chosen = regex ?? prefix;
  if (!chosen) throw new Error(`no location answers ${path}`);
  return chosen;
}

// A build: public/ is copied to the root, Vite adds the shell, the hashed
// assets and the MCP App views.
const BUILD = new Set([
  "/index.html",
  "/robots.txt",
  "/favicon.ico",
  "/icon.svg",
  "/manifest.webmanifest",
  "/sw.js",
  "/assets/index-3f9a1c.js",
  "/mcp-apps/deal-card.html",
]);

type Answer = "file" | "shell" | 404;

function answer(path: string): Answer {
  const where = locate(path);
  if (where.modifier === "=") return "file";
  const tries = /try_files\s+([^;]+);/.exec(where.body)?.[1].split(/\s+/);
  if (!tries) throw new Error(`the location for ${path} has no try_files`);
  // A built file is served only where nginx tries the path itself first.
  if (BUILD.has(path) && tries[0] === "$uri") return "file";
  const last = tries.at(-1);
  if (last === "=404") return 404;
  if (last === "/index.html") return "shell";
  throw new Error(`unexpected try_files fallback ${last}`);
}

const headers = (body: string) =>
  new Map(
    Array.from(
      body.matchAll(/add_header\s+(\S+)\s+"([^"]*)"\s+always;/g),
      (m) => [m[1], m[2]],
    ),
  );

describe("the web tier answers a file it does not have with 404, never the app shell", () => {
  it.each([
    "/wp-login.php",
    "/backup.zip",
    "/admin.php",
    "/llms.txt",
    "/security.txt",
    "/.well-known/security.txt",
    "/.env",
    "/.git/config",
    "/backup.zip/",
    "/wp-login.php/",
    "/.env/",
    "/apple-touch-icon-precomposed.png",
    "/assets/index-0000.js",
    "/mcp-apps/missing.html",
  ])("%s is a 404", (path) => {
    expect(answer(path)).toBe(404);
  });

  it.each(["/", "/does-not-exist", "/contacts", "/deals/pipeline"])(
    "%s, which names no file, falls back to the shell for the hash router",
    (path) => {
      expect(answer(path)).toBe("shell");
    },
  );

  it.each([...BUILD])("%s is served from the build", (path) => {
    expect(answer(path)).toBe("file");
  });

  it("keeps the cache policy of the asset and view prefixes, which no regex overrides", () => {
    expect(locate("/assets/index-3f9a1c.js").body).toContain(
      '"public, immutable"',
    );
    expect(locate("/mcp-apps/deal-card.html").body).toContain(
      "default-src 'none'",
    );
  });

  it("answers every 404 with a short plain-text body", () => {
    expect(conf).toMatch(/error_page\s+404\s+@not_found;/);
    const notFound = locations().find((l) => l.pattern === "@not_found");
    expect(notFound?.body).toMatch(/types\s*\{\s*\}/);
    expect(notFound?.body).toMatch(/default_type\s+text\/plain;/);
    expect(notFound?.body).toMatch(/return\s+404\s+"Not found\\n";/);
    expect(notFound?.body).not.toMatch(/add_header/);
    expect(conf).toMatch(/server_tokens\s+off;/);
  });
});

describe("every response the web tier sends carries the whole security set", () => {
  const server = headers(conf.slice(0, conf.search(/^\s*location\s/m)));

  it("declares the whole set on the server block", () => {
    for (const name of [
      "Content-Security-Policy",
      "X-Frame-Options",
      "X-Content-Type-Options",
      "X-Robots-Tag",
      "Referrer-Policy",
      "Strict-Transport-Security",
      "Permissions-Policy",
      "Cross-Origin-Opener-Policy",
      "Cross-Origin-Resource-Policy",
    ]) {
      expect(server.has(name), name).toBe(true);
    }
  });

  // The shell and the file-or-404 location inherit the set; one add_header of
  // their own would replace it, and every page load would ship with none.
  it("adds no header of its own where the shell and the 404s are answered", () => {
    for (const path of ["/", "/contacts", "/backup.zip"]) {
      expect(locate(path).body, path).not.toMatch(/add_header/);
    }
  });

  // add_header in a location REPLACES the inherited set, so a location that
  // declares any header must restate all of them.
  it("repeats the set, value for value, in each location that adds a header", () => {
    for (const where of locations().filter((l) => /add_header/.test(l.body))) {
      const own = headers(where.body);
      for (const [name, value] of server) {
        if (
          name === "Content-Security-Policy" &&
          where.pattern === "/mcp-apps/"
        ) {
          expect(own.get(name), "the views' own CSP").toBeDefined();
          continue;
        }
        expect(own.get(name), `${name} in location ${where.pattern}`).toBe(
          value,
        );
      }
    }
  });
});
