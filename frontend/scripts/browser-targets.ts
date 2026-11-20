// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { getAllVersions } from "baseline-browser-mapping";
import browserslist from "browserslist";
import { stable } from "../src/format/collate";

const FRONTEND = resolve(dirname(fileURLToPath(import.meta.url)), "..");

// Each caniuse browser Vite can lower a bundle for, by the name build.target
// spells it with. Vite also hands these to Lightning CSS as the CSS floor.
const VITE_ENGINES: Readonly<Record<string, string>> = {
  chrome: "chrome",
  and_chr: "chrome",
  edge: "edge",
  firefox: "firefox",
  safari: "safari",
  ios_saf: "ios",
  opera: "opera",
};

// Chromium builds Vite has no target name for, by the name
// baseline-browser-mapping gives each. It knows the Chromium each version runs,
// and that Chromium is what the build lowers for.
const CHROMIUM_BUILDS: Readonly<Record<string, string>> = {
  samsung: "samsunginternet_android",
  android: "webview_android",
  op_mob: "opera_android",
};

/** Each Chromium build's releases, oldest first, with the Chromium each runs. */
export type ChromiumReleases = ReadonlyMap<
  string,
  readonly { version: number[]; chromium: number }[]
>;

function chromiumReleases(): ChromiumReleases {
  const releases = new Map<string, { version: number[]; chromium: number }[]>();
  const all = getAllVersions({
    includeDownstreamBrowsers: true,
    outputFormat: "array",
  });
  if (!Array.isArray(all)) {
    throw new Error("baseline-browser-mapping answered no list of versions.");
  }
  for (const release of all) {
    if (release.engine !== "Blink" || release.engine_version === undefined) {
      continue;
    }
    const list = releases.get(release.browser) ?? [];
    list.push({
      version: release.version.split(".").map(Number),
      chromium: Number(release.engine_version),
    });
    releases.set(release.browser, list);
  }
  for (const list of releases.values()) {
    list.sort((a, b) => (older(a.version, b.version) ? -1 : 1));
  }
  return releases;
}

function versionParts(browser: string, version: string): number[] {
  const parts = version.split(".").map(Number);
  if (parts.some(Number.isNaN)) {
    throw new Error(
      `.browserslistrc resolves "${browser}", whose version the build cannot compare. Leave it out of the policy.`,
    );
  }
  return parts;
}

function older(a: readonly number[], b: readonly number[]): boolean {
  for (let i = 0; i < Math.max(a.length, b.length); i++) {
    const left = a[i] ?? 0;
    const right = b[i] ?? 0;
    if (left !== right) return left < right;
  }
  return false;
}

// The Chromium a build's version runs. A version newer than the mapping's data
// runs at least the newest Chromium the data knows, so that one is its floor.
function chromiumOf(
  browser: string,
  parts: readonly number[],
  known: readonly { version: number[]; chromium: number }[],
): number {
  const newest = known.at(-1);
  if (newest !== undefined && older(newest.version, parts)) {
    return newest.chromium;
  }
  const match = known.find(
    ({ version }) => !older(version, parts) && !older(parts, version),
  );
  if (match === undefined) {
    throw new Error(
      `.browserslistrc resolves "${browser}", whose Chromium baseline-browser-mapping does not know. Update baseline-browser-mapping, or leave it out of the policy.`,
    );
  }
  return match.chromium;
}

/** The oldest version of each engine in `browsers`, as Vite build targets. */
export function viteTargets(
  browsers: readonly string[],
  chromium: ChromiumReleases = chromiumReleases(),
): string[] {
  const floors = new Map<string, { version: string; parts: number[] }>();
  for (const browser of browsers) {
    const [name = "", range = ""] = browser.split(" ");
    let engine = VITE_ENGINES[name];
    let version = range.split("-")[0] ?? range;
    let parts = versionParts(browser, version);
    const build = CHROMIUM_BUILDS[name];
    if (build !== undefined) {
      const runs = chromiumOf(browser, parts, chromium.get(build) ?? []);
      engine = "chrome";
      version = String(runs);
      parts = [runs];
    }
    if (engine === undefined) {
      throw new Error(
        `.browserslistrc resolves "${browser}", which Vite cannot build for. Name it in scripts/browser-targets.ts or leave it out of the policy.`,
      );
    }
    const known = floors.get(engine);
    if (known === undefined || older(parts, known.parts)) {
      floors.set(engine, { version, parts });
    }
  }
  return [...floors]
    .map(([engine, { version }]) => `${engine}${version}`)
    .sort(stable);
}

/** The browsers .browserslistrc supports, as caniuse names and versions. */
export function supportedBrowsers(): string[] {
  return browserslist(undefined, { path: FRONTEND, env: "production" });
}

/** The build.target that lowers the bundle to every supported browser. */
export function policyTargets(): string[] {
  return viteTargets(supportedBrowsers());
}
