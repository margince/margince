// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import browserslist from "browserslist";

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

// Chromium builds Vite has no target name for. Each runs the engine of a
// recent Chrome, so the Chrome floor is the one the bundle is lowered for.
const CHROMIUM_BUILDS: ReadonlySet<string> = new Set([
  "samsung",
  "and_uc",
  "and_qq",
  "android",
  "op_mob",
]);

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

/** The oldest version of each engine in `browsers`, as Vite build targets. */
export function viteTargets(browsers: readonly string[]): string[] {
  const floors = new Map<string, { version: string; parts: number[] }>();
  for (const browser of browsers) {
    const [name = "", range = ""] = browser.split(" ");
    if (CHROMIUM_BUILDS.has(name)) continue;
    const engine = VITE_ENGINES[name];
    if (engine === undefined) {
      throw new Error(
        `.browserslistrc resolves "${browser}", which Vite cannot build for. Name it in scripts/browser-targets.ts or leave it out of the policy.`,
      );
    }
    const version = range.split("-")[0] ?? range;
    const parts = versionParts(browser, version);
    const known = floors.get(engine);
    if (known === undefined || older(parts, known.parts)) {
      floors.set(engine, { version, parts });
    }
  }
  return [...floors]
    .map(([engine, { version }]) => `${engine}${version}`)
    .sort();
}

/** The browsers .browserslistrc supports, as caniuse names and versions. */
export function supportedBrowsers(): string[] {
  return browserslist(undefined, { path: FRONTEND, env: "production" });
}

/** The build.target that lowers the bundle to every supported browser. */
export function policyTargets(): string[] {
  return viteTargets(supportedBrowsers());
}
