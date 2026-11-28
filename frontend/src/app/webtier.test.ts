// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { mkdtemp, readFile, realpath, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { build } from "vite";
import { describe, expect, it } from "vitest";
import { MIN_BYTES, PRECOMPRESSED_TYPES } from "../../scripts/vite-precompress";
import { BOOKING_HOST_IDS, BOOKING_ID_PREFIXES } from "../lib/bookingaddress";
import { CONSENT_PATH, EMBED_PATH } from "../lib/recordaddress";

// What nginx.conf answers for a path, worked out from the file the image ships
// by nginx's own location rules: an exact match first, then the longest
// prefix, which ends the search when it is ^~, then the regexes in file order,
// then that longest prefix.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const conf = readFileSync(join(frontendRoot, "nginx.conf"), "utf8")
  .split("\n")
  .map((line) => line.replace(/^\s*#.*$/, ""))
  .join("\n");

// The app's icon, as the shell names it. The Dispact app manifest and the MCP
// handshake hand this same path to pages on other origins.
const ICON_PATH = /<link rel="icon" type="image\/svg\+xml" href="([^"]+)"/.exec(
  readFileSync(join(frontendRoot, "index.html"), "utf8"),
)?.[1];

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

  // The pdf.js worker that reads a Voice DNA PDF is a hashed .mjs asset. A
  // browser runs a module worker only when it arrives typed as JavaScript.
  it("serves a hashed .mjs asset as JavaScript", () => {
    const assets = locate("/assets/pdf.worker.min-3f9a1c.mjs").body;
    const mjs = /location\s+~\s+\\\.mjs\$\s*\{([^}]*\{\s*\}[^}]*)\}/.exec(
      assets,
    )?.[1];
    expect(mjs, "a .mjs location inside /assets/").toBeDefined();
    expect(mjs).toMatch(/types\s*\{\s*\}/);
    expect(mjs).toMatch(/default_type\s+text\/javascript;/);
    expect(mjs).toMatch(/try_files\s+\$uri\s+=404;/);
    expect(mjs).not.toMatch(/add_header/);
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
        if (
          name === "Cross-Origin-Resource-Policy" &&
          where.pattern === ICON_PATH
        ) {
          expect(own.get(name), "the icon's own CORP").toBe("cross-origin");
          continue;
        }
        if (where.pattern === EMBED_PATH && name === "X-Frame-Options") {
          expect(
            own.get(name),
            "the embed path's frame-ancestors decides",
          ).toBe(undefined);
          continue;
        }
        if (
          where.pattern === EMBED_PATH &&
          name === "Content-Security-Policy"
        ) {
          expect(own.get(name), "the embed path's own frame-ancestors").toBe(
            value.replace(
              "frame-ancestors 'none'",
              "frame-ancestors $embed_ancestors",
            ),
          );
          continue;
        }
        if (
          name === "Cross-Origin-Opener-Policy" &&
          where.pattern === CONSENT_PATH
        ) {
          expect(own.get(name), "the consent screen's own COOP").toBe(
            "unsafe-none",
          );
          continue;
        }
        expect(own.get(name), `${name} in location ${where.pattern}`).toBe(
          value,
        );
      }
    }
  });
});

// The embed path is the one page another origin may frame, and only an origin
// the operator names in /etc/nginx/margince/. With no such file it refuses
// every frame, as every other page does.
it("lets only the embed path name frame ancestors, and none until an operator does", () => {
  const where = locate(EMBED_PATH);
  expect(where.modifier).toBe("=");
  expect(where.body).toContain("try_files /index.html =404;");
  const defaulted = where.body.indexOf(`set $embed_ancestors "'none'";`);
  const included = where.body.indexOf("include /etc/nginx/margince/*.conf;");
  expect(defaulted, "the default").toBeGreaterThan(-1);
  expect(
    included,
    "the operator's file, read after the default",
  ).toBeGreaterThan(defaulted);
  for (const other of locations().filter((l) => l.pattern !== EMBED_PATH)) {
    expect(other.body, other.pattern).not.toContain("$embed_ancestors");
  }
  for (const path of ["/", "/embed", "/embed/x", "/index.html", "/consent"]) {
    expect(locate(path).pattern, path).not.toBe(EMBED_PATH);
  }
});

// Another origin's page shows the icon in an <img>, which a same-origin CORP
// blocks. Only the icon is opened up; the shell and the assets stay closed.
it("lets another origin load the app's icon, and nothing else", () => {
  expect(ICON_PATH, "the shell names its icon").toBe("/icon.svg");
  const where = locate(ICON_PATH ?? "");
  expect(where.modifier).toBe("=");
  expect(where.body).toContain("try_files $uri =404;");
  const own = headers(where.body);
  expect(own.get("Cross-Origin-Resource-Policy")).toBe("cross-origin");
  expect(own.get("Access-Control-Allow-Origin")).toBe("*");
  const server = headers(conf.slice(0, conf.search(/^\s*location\s/m)));
  for (const path of ["/", "/index.html", "/assets/app.js", "/icon.png"]) {
    expect(
      headers(locate(path).body).get("Cross-Origin-Resource-Policy") ??
        server.get("Cross-Origin-Resource-Policy"),
      path,
    ).toBe("same-origin");
  }
});

// The app's consent path is the one nginx serves with a popup's opener kept.
it("serves the consent screen as the shell, leaving a popup its opener", () => {
  const server = headers(conf.slice(0, conf.search(/^\s*location\s/m)));
  const where = locate(CONSENT_PATH);
  expect(where.modifier).toBe("=");
  expect(where.body).toContain("try_files /index.html =404;");
  expect(headers(where.body).get("Cross-Origin-Opener-Policy")).toBe(
    "unsafe-none",
  );
  for (const path of ["/", "/consent/", "/consentx", "/index.html"]) {
    expect(
      headers(locate(path).body).get("Cross-Origin-Opener-Policy") ??
        server.get("Cross-Origin-Opener-Policy"),
      path,
    ).toBe("same-origin");
  }
});

it("routes a record card to the public metadata endpoint rather than the static shell", () => {
  for (const kind of ["deals", "companies", "contacts"]) {
    expect(
      locate(`/r/${kind}/01a00000-0000-7000-8000-000000000001`).body,
    ).toContain("return 302 /v1/public/record-previews/$1/$2;");
  }
  expect(locate("/open-record.js").body).toContain("expires -1;");
});

describe("a booking page has a real path a preview fetcher can read", () => {
  const SLUG = "Ab3_x-9QrStUvWxYz0123456789abcd";

  it.each([
    `/book/${SLUG}`,
    `/book/${SLUG}/`,
    "/book/lowercase-and_UPPER",
    // A host's short address, in each case a guest may type it.
    "/book/alex",
    "/book/ALEX",
    "/book/alex-sales/",
  ])(
    "sends every fetch of %s, a browser's navigation and a preview fetcher's alike, to the booking card",
    (path) => {
      const where = locate(path);
      expect(where.pattern).toBe("^/book/([A-Za-z0-9_-]+)/?$");
      expect(where.body).toMatch(
        /^\s*return 302 \/v1\/public\/booking\/\$1\/preview\$is_args\$args;\s*$/m,
      );
      // No branch on the request: the shell's head names Margince, so no
      // header variant may reach it here.
      expect(where.body).not.toMatch(/\bif\s*\(|try_files|sec_fetch_dest/);
      expect(headers(where.body).get("Cache-Control")).toBe("no-store");
      expect(headers(where.body).has("Vary")).toBe(false);
    },
  );

  it.each([
    "/book/x.png",
    "/book/slug.json/",
    "/book/.env",
    "/book/a/b",
    "/book/",
    "/book/alex.sales",
  ])(
    "refuses %s, which names no booking page, with an unlogged 404",
    (path) => {
      const where = locate(path);
      expect(where.pattern).toBe("^/book/");
      expect(where.body).not.toContain("/v1/public/booking/");
      expect(where.body).toMatch(/access_log off;/);
      expect(where.body).toMatch(/return 404 "Not found\\n";/);
    },
  );

  it.each(["/Book/abc", "/book"])(
    "leaves %s, which is outside /book/, to the shell",
    (path) => {
      expect(locate(path).body).not.toContain("/v1/public/booking/");
      expect(answer(path)).toBe("shell");
    },
  );
});

describe("a booking path that names a host control or a guest token is refused unlogged", () => {
  const reserved = [
    ...BOOKING_HOST_IDS,
    ...BOOKING_ID_PREFIXES.map((prefix) => `${prefix}private-token`),
  ];

  it.each(reserved.flatMap((id) => [`/book/${id}`, `/book/${id}/`]))(
    "answers %s with 404 and never sends it to the preview",
    (path) => {
      const where = locate(path);
      expect(where.body).not.toContain("/v1/public/booking/");
      expect(where.body).toMatch(/return 404 "Not found\\n";/);
      expect(where.body).toMatch(/access_log off;/);
    },
  );

  it("leaves a slug that only starts like a reserved word to the booking page", () => {
    for (const path of ["/book/managex", "/book/previews", "/book/contacts"]) {
      expect(locate(path).pattern).toBe("^/book/([A-Za-z0-9_-]+)/?$");
    }
  });

  it("does not log a booking page's slug", () => {
    expect(locate("/book/Ab3_x-9Q").body).toMatch(/access_log off;/);
  });
});

describe("the web tier sends the build's precompressed files", () => {
  const directive = (name: string) =>
    new RegExp(`^    ${name}\\s+([^;]+);`, "m").exec(conf)?.[1];

  it("serves a .gz the build wrote, and compresses the rest at level 6 behind any proxy", () => {
    expect(directive("gzip")).toBe("on");
    expect(directive("gzip_static")).toBe("on");
    expect(directive("gzip_comp_level")).toBe("6");
    expect(directive("gzip_proxied")).toBe("any");
    expect(directive("gzip_vary")).toBe("on");
    expect(conf).not.toMatch(/gzip(?:_static)?\s+off;/);
  });

  // Below the floor nginx sends a file as it is, so a .gz there is never read.
  it("skips the files nginx would not compress", () => {
    expect(Number(directive("gzip_min_length"))).toBe(MIN_BYTES);
  });

  it("precompresses every type nginx compresses on the fly", () => {
    const types = directive("gzip_types")?.split(/\s+/) ?? [];
    expect(types).not.toHaveLength(0);
    const written = new Set(Object.values(PRECOMPRESSED_TYPES));
    expect(types.filter((type) => !written.has(type))).toEqual([]);
    expect(written.has("text/html")).toBe(true);
  });
});

describe("the content policy admits the shell's inline theme script and nothing else", () => {
  const shell = readFileSync(join(frontendRoot, "index.html"), "utf8");
  const inline = [...shell.matchAll(/<script>([\s\S]*?)<\/script>/g)].map(
    (match) => match[1],
  );
  const digest = (script: string) =>
    `'sha256-${createHash("sha256").update(script).digest("base64")}'`;
  const scriptSources = (csp: string) =>
    /(?:^|;\s*)script-src\s+([^;]+)/.exec(csp)?.[1].split(/\s+/);

  it("names the script's sha256 in every policy that sets script-src", () => {
    expect(inline).toHaveLength(1);
    const policies = [
      ...conf.matchAll(/add_header\s+Content-Security-Policy\s+"([^"]*)"/g),
    ].map((match) => match[1]);
    expect(policies.filter((csp) => scriptSources(csp))).not.toHaveLength(0);
    for (const csp of policies) {
      const sources = scriptSources(csp);
      if (sources === undefined) continue;
      expect(sources, csp).toEqual(["'self'", digest(inline[0])]);
    }
  });

  // The policy hashes index.html's bytes, so the build must emit them as written.
  it("leaves the script's bytes as written in the built shell", async () => {
    const dir = await realpath(await mkdtemp(join(tmpdir(), "inline-script-")));
    try {
      await writeFile(
        join(dir, "index.html"),
        `<!doctype html><html><head><script>${inline[0]}</script></head><body><script type="module" src="./entry.js"></script></body></html>`,
      );
      await writeFile(join(dir, "entry.js"), "console.log(1);");
      await build({
        root: dir,
        configFile: false,
        logLevel: "silent",
        build: { outDir: join(dir, "dist") },
      });
      const built = await readFile(join(dir, "dist", "index.html"), "utf8");
      expect(
        [...built.matchAll(/<script>([\s\S]*?)<\/script>/g)].map((m) =>
          digest(m[1]),
        ),
      ).toEqual([digest(inline[0])]);
    } finally {
      await rm(dir, { recursive: true, force: true });
    }
  });
});
