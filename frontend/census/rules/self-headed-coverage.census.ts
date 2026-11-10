// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A page has one h1. The shell's `PageTitle` names the page unless the screen
// is in `SELF_HEADED_SCREENS`, where it stands down. A screen that heads itself
// and is not in the set gives two page titles; one listed and silent gives none.
//
// The tell is `usePageName("<screen>")`: the hook exists only to print the name
// the shell would have printed, so calling it is the claim to head yourself.

import { SELF_HEADED_SCREENS } from "../../src/app/pagemeta";
import type { CensusFile, CensusRule } from "../tree";
import { screenCorpus } from "./screenfiles.census";

// The screens that head themselves without the hook, each with its reason: a
// heading built from something the shell cannot resolve from the route.
const HEADS_ITSELF_WITHOUT_THE_HOOK: Readonly<Record<string, string>> = {
  // The greeting is the heading, and it holds the reader's name.
  home: "greets the reader by name",
  tags: "heads itself with the tag",
  lists: "heads itself with the list",
};

/** Every screen the `usePageName` hook is called in, by the route it names. */
export function screensThatNameThemselves(
  corpus: readonly CensusFile[],
): Set<string> {
  const named = new Set<string>();
  for (const file of corpus) {
    for (const [, screen] of file.text.matchAll(/usePageName\("([^"]+)"\)/g)) {
      named.add(screen);
    }
  }
  return named;
}

/** Screens that head themselves unlisted, then listed screens that are silent. */
export function headingFindings(
  named: ReadonlySet<string>,
  selfHeaded: ReadonlySet<string>,
): string[] {
  const missing = [...named]
    .filter((screen) => !selfHeaded.has(screen))
    .map(
      (screen) =>
        `${screen} prints the page name and is not in SELF_HEADED_SCREENS`,
    );
  const silent = [...selfHeaded]
    .filter(
      (screen) =>
        !named.has(screen) &&
        HEADS_ITSELF_WITHOUT_THE_HOOK[screen] === undefined,
    )
    .map(
      (screen) => `${screen} is in SELF_HEADED_SCREENS and prints no page name`,
    );
  return [...missing, ...silent];
}

export const rule: CensusRule = {
  name: "self-headed-coverage",
  corpus: screenCorpus,
  // A regex that stopped matching would find nothing missing and pass.
  floors: (corpus) =>
    screensThatNameThemselves(corpus).size >= 6
      ? []
      : ["fewer than 6 screens call usePageName"],
  across: (corpus) =>
    headingFindings(screensThatNameThemselves(corpus), SELF_HEADED_SCREENS),
  advice:
    "A screen that prints the page's name is one the shell stands down for, " +
    "and the other way round. Add it to SELF_HEADED_SCREENS in " +
    "app/pagemeta.ts, or drop it from the set.",
};
