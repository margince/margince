// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A `mailto:` built from an address sends the message around the composer.
// Only ContactLink may build one, for a reader with no mailbox. Every product
// source file is read, so a new surface cannot slip by.

import type { CensusFile, CensusRule } from "../tree";

export const BUILDS_MAILTO = /mailto:\$\{/;

export const BUILDERS: ReadonlySet<string> = new Set([
  "src/design-system/contactlink.tsx",
  // A link the writer puts in their own message, opened by the recipient.
  "src/design-system/richtext-html.ts",
]);

const isProductSource = (file: CensusFile) =>
  file.area === "src" &&
  /\.tsx?$/.test(file.rel) &&
  !/\.(test|stories)\.tsx?$/.test(file.rel);

export const rule: CensusRule = {
  name: "contactlink",
  corpus: (tree) => tree.files.filter(isProductSource),
  across: (corpus) => {
    const builders = new Set(
      corpus.filter((file) => BUILDS_MAILTO.test(file.text)).map((f) => f.rel),
    );
    return [
      ...[...builders]
        .filter((rel) => !BUILDERS.has(rel))
        .map((rel) => `${rel} builds a mailto from an address`),
      ...[...BUILDERS]
        .filter((rel) => !builders.has(rel))
        .map((rel) => `${rel} is listed as a mailto builder and builds none`),
    ];
  },
  advice:
    "Render the address through ContactLink (src/design-system/contactlink.tsx), " +
    "which opens the composer and builds a mailto only for a reader with no " +
    "mailbox. A listed builder that no longer builds one leaves the list.",
};
