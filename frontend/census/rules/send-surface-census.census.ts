// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Every surface that sends says what the message would meet. A file that posts
// a send renders the engine's answer, or names itself in SILENT_SENDERS with
// why it does not.
//
// The failure is invisible at the call site: `api.POST("/emails", …)` reads
// the same whether the surface tells the rep anything or not.

import { SCREEN_ROOTS } from "../../src/testing/screenfiles";
import type { CensusFile, CensusRule } from "../tree";

// The three doors a send goes out of, as the string a caller types. A surface
// that builds the path some other way is one this census cannot see.
const SEND_PATHS = [
  '"/emails"',
  '"/activities/{id}/send-email"',
  '"/activities/{id}/send-message"',
];

// Any of these means the file draws the engine's answer before the rep presses.
const SAYS_SOMETHING = [
  "SendMark",
  "SendPermission",
  "CommunicationStatus",
  "useSendPermission",
];

// Files that post a send and draw no mark, each with the reason somebody
// wrote for it.
export const SILENT_SENDERS: Readonly<Record<string, string>> = {};

export const postsASend = (source: string): boolean =>
  SEND_PATHS.some((path) => source.includes(path));

const shown = (file: CensusFile) =>
  file.path.slice(file.path.indexOf("/src/") + 1);

/** The silent senders, then the waivers whose file is gone. */
export function silentSenders(
  corpus: readonly CensusFile[],
  waived: Readonly<Record<string, string>>,
): string[] {
  const silent = corpus
    .filter(
      (file) =>
        postsASend(file.text) &&
        !SAYS_SOMETHING.some((marker) => file.text.includes(marker)) &&
        !waived[shown(file)],
    )
    .map((file) => `${shown(file)} posts a send and draws no mark`);
  const present = new Set(corpus.map(shown));
  const stale = Object.keys(waived)
    .filter((named) => !present.has(named))
    .map((named) => `${named} is in SILENT_SENDERS and is gone`);
  return [...silent, ...stale];
}

export const rule: CensusRule = {
  name: "send-surface-census",
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        file.source !== undefined &&
        (file.area === "extensions" ||
          (file.area === "src" &&
            SCREEN_ROOTS.some((root) =>
              file.rel.startsWith(`src/${root}/`),
            ))) &&
        !/\.(test|stories)\.tsx?$/.test(file.path),
    ),
  // The composer posts all three doors, so a census finding no sender has
  // stopped finding its subject.
  floors: (corpus) =>
    corpus.some((file) => postsASend(file.text))
      ? []
      : ["no file posts a send"],
  across: (corpus) => silentSenders(corpus, SILENT_SENDERS),
  advice:
    "A surface posts a send and shows the rep nothing about what the message " +
    "would meet. Render SendMark or useSendPermission beside it, or name the " +
    "file in SILENT_SENDERS with the reason.",
};
