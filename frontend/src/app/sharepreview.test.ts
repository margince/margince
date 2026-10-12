// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync, statSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const read = (file: string) => readFileSync(join(frontendRoot, file), "utf8");

// Not read off robots.txt: a fetcher deleted there would leave this list too.
const PREVIEW_FETCHERS: readonly string[] = [
  "Twitterbot",
  "LinkedInBot",
  "Slackbot-LinkExpanding",
  "facebookexternalhit",
  "Discordbot",
  "TelegramBot",
  "WhatsApp",
  "SkypeUriPreview",
  "MicrosoftPreview",
];

// The AI crawlers and training-data tokens robots.txt refuses by name.
const AI_CRAWLERS: readonly string[] = [
  "GPTBot",
  "ChatGPT-User",
  "ClaudeBot",
  "Claude-Web",
  "anthropic-ai",
  "Google-Extended",
  "CCBot",
  "PerplexityBot",
  "Bytespider",
  "Applebot-Extended",
  "meta-externalagent",
  "cohere-ai",
  "Amazonbot",
];

type Attribute = "property" | "name";
type Meta = { attribute: Attribute; key: string; content: string };
type Rule = { allow: boolean; path: string };
type RobotsGroup = { agents: string[]; rules: Rule[] };
type Icon = { declaredBy: string; url: URL; sizes?: string };

const ATTRIBUTES: readonly Attribute[] = ["property", "name"];
const ICON_RELS = new Set(["icon", "apple-touch-icon", "manifest"]);
const ORIGIN = "https://installation.invalid";
// nginx serves the shell at any unknown path; relative hrefs resolve from there.
const SHELL_AT_UNKNOWN_PATH = `${ORIGIN}/no/such/path`;
const ATTRIBUTE_VALUE =
  /(?:^|\s)([\w:-]+)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>`]+))/g;
const PNG_SIGNATURE = Buffer.from([
  0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
]);

function headTags(element: "meta" | "link"): Map<string, string>[] {
  const html = read("index.html").replace(/<!--[\s\S]*?-->/g, "");
  const head = /<head\b[^>]*>([\s\S]*?)<\/head>/i.exec(html);
  if (!head) throw new Error("index.html has no <head>");
  return Array.from(
    head[1].matchAll(new RegExp(`<${element}\\b([^>]*)>`, "gi")),
    ([, attributes]) =>
      new Map(
        Array.from(
          attributes.matchAll(ATTRIBUTE_VALUE),
          (pair): [string, string] => [
            pair[1].toLowerCase(),
            pair[2] ?? pair[3] ?? pair[4],
          ],
        ),
      ),
  );
}

function headMetas(): Meta[] {
  const metas: Meta[] = [];
  for (const values of headTags("meta")) {
    for (const attribute of ATTRIBUTES) {
      const key = values.get(attribute);
      if (key !== undefined) {
        metas.push({
          attribute,
          key: key.toLowerCase(),
          content: values.get("content") ?? "",
        });
      }
    }
  }
  if (!metas.some((meta) => meta.key.startsWith("og:"))) {
    throw new Error("index.html declares no og: tag in its <head>");
  }
  return metas;
}

function tags(attribute: Attribute, key: string): string[] {
  return headMetas()
    .filter((meta) => meta.attribute === attribute && meta.key === key)
    .map((meta) => meta.content);
}

function tag(attribute: Attribute, key: string): string {
  const found = tags(attribute, key);
  if (found.length !== 1) {
    throw new Error(
      `index.html declares <meta ${attribute}="${key}"> ${found.length} times, not once`,
    );
  }
  return found[0];
}

function fields(value: unknown, what: string): Map<string, unknown> {
  if (typeof value !== "object" || value === null) {
    throw new Error(`${what} is not a JSON object`);
  }
  return new Map(Object.entries(value));
}

const manifest = () =>
  fields(JSON.parse(read("public/manifest.webmanifest")), "the manifest");

function headIconLinks(): Icon[] {
  const links = headTags("link").filter((values) =>
    (values.get("rel") ?? "")
      .toLowerCase()
      .split(/\s+/)
      .some((token) => ICON_RELS.has(token)),
  );
  if (links.length === 0) {
    throw new Error("index.html links no icon and no manifest");
  }
  return links.map((values) => ({
    declaredBy: `index.html <link rel="${values.get("rel")}">`,
    url: new URL(values.get("href") ?? "", SHELL_AT_UNKNOWN_PATH),
    sizes: values.get("sizes"),
  }));
}

function manifestIcons(): Icon[] {
  const icons = manifest().get("icons");
  if (!Array.isArray(icons) || icons.length === 0) {
    throw new Error("the manifest lists no icons");
  }
  return icons.map((icon: unknown, index) => {
    const declaredBy = `manifest icons[${index}]`;
    const entry = fields(icon, declaredBy);
    const src = entry.get("src");
    const sizes = entry.get("sizes");
    if (typeof src !== "string") throw new Error(`${declaredBy} has no src`);
    return {
      declaredBy,
      url: new URL(src, `${ORIGIN}/manifest.webmanifest`),
      sizes: typeof sizes === "string" ? sizes : undefined,
    };
  });
}

function publicFile(icon: Icon): string {
  if (icon.url.origin !== ORIGIN) {
    throw new Error(`${icon.declaredBy} points off-origin at ${icon.url}`);
  }
  return join(frontendRoot, "public", icon.url.pathname);
}

function pngSize(file: string): string {
  const bytes = readFileSync(file);
  if (
    !bytes.subarray(0, 8).equals(PNG_SIGNATURE) ||
    bytes.toString("latin1", 12, 16) !== "IHDR"
  ) {
    throw new Error(`${file} is not a PNG`);
  }
  return `${bytes.readUInt32BE(16)}x${bytes.readUInt32BE(20)}`;
}

function robotsGroups(): RobotsGroup[] {
  const groups: RobotsGroup[] = [];
  for (const line of read("public/robots.txt").split("\n")) {
    const record = /^([A-Za-z-]+)\s*:\s*(.*)$/.exec(
      line.replace(/#.*/, "").trim(),
    );
    if (!record) continue;
    const field = record[1].toLowerCase();
    const value = record[2].trim();
    const current = groups.at(-1);
    if (field === "user-agent") {
      if (current && current.rules.length === 0) {
        current.agents.push(value.toLowerCase());
      } else {
        groups.push({ agents: [value.toLowerCase()], rules: [] });
      }
    } else if ((field === "allow" || field === "disallow") && current) {
      current.rules.push({ allow: field === "allow", path: value });
    }
  }
  if (groups.length === 0) throw new Error("robots.txt parses to no group");
  return groups;
}

function matches(pattern: string, path: string): boolean {
  const anchored = pattern.endsWith("$");
  const body = (anchored ? pattern.slice(0, -1) : pattern)
    .split("*")
    .map((part) => part.replace(/[.+?^${}()|[\]\\]/g, "\\$&"))
    .join(".*");
  return new RegExp(`^${body}${anchored ? "$" : ""}`).test(path);
}

// RFC 9309's verdict, not rule presence: a longer Disallow beats an Allow.
function allowed(agent: string, path: string): boolean {
  const groups = robotsGroups();
  const named = groups.filter((group) =>
    group.agents.includes(agent.toLowerCase()),
  );
  const governing =
    named.length > 0
      ? named
      : groups.filter((group) => group.agents.includes("*"));
  let verdict: Rule | undefined;
  for (const rule of governing.flatMap((group) => group.rules)) {
    if (rule.path === "" || !matches(rule.path, path)) continue;
    const longer = !verdict || rule.path.length > verdict.path.length;
    const tieToAllow = verdict?.path.length === rule.path.length && rule.allow;
    if (longer || tieToAllow) verdict = rule;
  }
  return verdict?.allow ?? true;
}

describe("a shared Margince link unfurls without running the app", () => {
  it("carries every tag a preview card is drawn from", () => {
    for (const key of ["og:title", "og:description", "og:type"]) {
      expect(tag("property", key), key).not.toBe("");
    }
    expect(tags("property", "og:image").length).toBeGreaterThan(0);
    expect(tag("name", "twitter:card")).not.toBe("");
  });

  it("names its image by an absolute https URL, since an installation's host is unknown", () => {
    const images = tags("property", "og:image");
    expect(images.length).toBeGreaterThan(0);
    for (const image of images) {
      expect(new URL(image).protocol, image).toBe("https:");
    }
  });

  it("declares no og:url, since no build knows an installation's address", () => {
    expect(headMetas().filter((meta) => meta.key === "og:url")).toEqual([]);
  });

  it("gives the description, og:description and the manifest one tagline", () => {
    expect(tag("name", "description")).not.toBe("");
    expect(tag("property", "og:description")).toBe(tag("name", "description"));
    expect(manifest().get("description")).toBe(tag("name", "description"));
  });
});

describe("every icon the shell and the manifest name ships at the size it claims", () => {
  it("resolves each icon and manifest link to a file under public/", () => {
    for (const icon of [...headIconLinks(), ...manifestIcons()]) {
      const file = statSync(publicFile(icon), { throwIfNoEntry: false });
      expect(file?.isFile(), `${icon.declaredBy} → ${icon.url}`).toBe(true);
    }
  });

  it("declares each PNG at the width and height its IHDR records", () => {
    const pngs = [...headIconLinks(), ...manifestIcons()].filter((icon) =>
      icon.url.pathname.endsWith(".png"),
    );
    expect(pngs.length).toBeGreaterThan(0);
    for (const png of pngs) {
      expect(pngSize(publicFile(png)), png.declaredBy).toBe(
        png.sizes?.toLowerCase(),
      );
    }
  });
});

describe("robots.txt lets link previews through and nothing else", () => {
  it.each(PREVIEW_FETCHERS)("lets %s fetch the shared page", (agent) => {
    expect(allowed(agent, "/")).toBe(true);
  });

  it("names no crawler outside the preview fetchers and the refused AI crawlers", () => {
    const named = new Set(
      [...PREVIEW_FETCHERS, ...AI_CRAWLERS].map((agent) => agent.toLowerCase()),
    );
    const others = robotsGroups()
      .flatMap((group) => group.agents)
      .filter((agent) => agent !== "*" && !named.has(agent));
    expect(others).toEqual([]);
  });

  it.each(AI_CRAWLERS)("refuses %s by name, everywhere", (agent) => {
    const naming = robotsGroups().filter((group) =>
      group.agents.includes(agent.toLowerCase()),
    );
    expect(naming).toHaveLength(1);
    expect(naming[0].rules).toEqual([{ allow: false, path: "/" }]);
    expect(allowed(agent, "/")).toBe(false);
    expect(allowed(agent, "/assets/app.js")).toBe(false);
  });

  it("asks every other crawler not to fetch at all", () => {
    expect(allowed("Googlebot", "/")).toBe(false);
  });
});
