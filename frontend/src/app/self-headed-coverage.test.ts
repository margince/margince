// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { SELF_HEADED_SCREENS } from "./pagemeta";

// Fitness function for the ONE h1 a page is allowed.
//
// Two surfaces can name a page: the shell's `PageTitle`, above the content, and
// a screen that prints its own heading. `SELF_HEADED_SCREENS` is what keeps
// them from both doing it — the shell reads it and stands down. So a screen
// that starts heading itself and forgets the set gives the document two page
// titles, and a screen that leaves the set while still listed gives it none.
// Neither shows up as a broken build and both are invisible in a screenshot.
//
// The tell is `usePageName("<screen>")`: the hook exists only to let a screen
// print the name the shell would have printed, so calling it IS the claim to
// head yourself, and the argument names the route the claim is about. The
// census is the screens directory rather than a list here, so a new
// self-heading screen is covered the day it is written.
//
// The two screens with no such call are named below with their own reason, and
// they are the only exemption: a heading built from something the shell cannot
// resolve — the reader's name, a tag's name — is not a page name lookup.
const HEADS_ITSELF_WITHOUT_THE_HOOK: Readonly<Record<string, string>> = {
  // "Guten Morgen, Demo." — the greeting is the heading, and it is a person's
  // name rather than the page's.
  home: "greets the reader by name",
  // The tag's own name, which the shell cannot know from the route alone.
  tags: "heads itself with the tag",
};

const screensDir = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "../screens",
);

/** Every screen the `usePageName` hook is called in, by the route it names. */
function screensThatNameThemselves(): Set<string> {
  const named = new Set<string>();
  for (const file of readdirSync(screensDir)) {
    if (
      !file.endsWith(".tsx") ||
      file.includes(".test.") ||
      file.includes(".stories.")
    ) {
      continue;
    }
    const source = readFileSync(resolve(screensDir, file), "utf8");
    for (const [, screen] of source.matchAll(/usePageName\("([^"]+)"\)/g)) {
      named.add(screen);
    }
  }
  return named;
}

describe("a page is named exactly once", () => {
  it("every screen that prints the page's name is one the shell stands down for", () => {
    const missing = [...screensThatNameThemselves()].filter(
      (screen) => !SELF_HEADED_SCREENS.has(screen),
    );
    expect(missing).toEqual([]);
  });

  it("every screen the shell stands down for prints the name itself", () => {
    const named = screensThatNameThemselves();
    const silent = [...SELF_HEADED_SCREENS].filter(
      (screen) =>
        !named.has(screen) &&
        HEADS_ITSELF_WITHOUT_THE_HOOK[screen] === undefined,
    );
    expect(silent).toEqual([]);
  });

  // The census must not be able to fail short: a regex that stopped matching
  // would report an empty set, find nothing missing, and pass. So it has to
  // find the screens that are there today.
  it("finds the screens that head themselves", () => {
    expect(screensThatNameThemselves().size).toBeGreaterThanOrEqual(6);
  });
});
