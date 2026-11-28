// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The Ctrl cap is copy: a German keyboard prints "Strg". So the shipped code
// never spells "Ctrl" itself. The keymap takes the word from the catalog
// (`shell.shortcuts.ctrl`), and every hint draws its caps from the keymap.
//
// The subject is a string the code could put on screen: a string literal, a
// template's text, or JSX text. Comments are not read, and neither are tests
// and stories, which name the key to drive it.

import ts from "typescript";
import { mayHoldLiteral } from "../../scripts/lib/literal-text";
import type { CensusFile, CensusRule } from "../tree";

const CTRL = /(?<![A-Za-z])Ctrl(?![A-Za-z])/;

/** Whether a file can carry the offence: every match has the word in it. */
export function maySpellCtrl(text: string): boolean {
  return mayHoldLiteral(text, "Ctrl");
}

function textOf(node: ts.Node): string | null {
  if (
    ts.isStringLiteral(node) ||
    ts.isNoSubstitutionTemplateLiteral(node) ||
    ts.isTemplateHead(node) ||
    ts.isTemplateMiddle(node) ||
    ts.isTemplateTail(node) ||
    ts.isJsxText(node)
  ) {
    return node.text;
  }
  return null;
}

/** Every line in `source` whose shown text spells Ctrl. */
export function ctrlSpellingsIn(source: ts.SourceFile): number[] {
  const lines: number[] = [];
  const visit = (node: ts.Node) => {
    const text = textOf(node);
    if (text !== null && CTRL.test(text)) {
      lines.push(
        source.getLineAndCharacterOfPosition(node.getStart()).line + 1,
      );
    }
    ts.forEachChild(node, visit);
  };
  ts.forEachChild(source, visit);
  return lines;
}

const DRIVES_THE_KEY = /\.(?:test|stories)\.tsx?$/;

const sitesIn = (file: CensusFile): string[] =>
  file.source && !DRIVES_THE_KEY.test(file.rel) && maySpellCtrl(file.text)
    ? ctrlSpellingsIn(file.source).map((line) => `${file.rel}:${line}`)
    : [];

export const rule: CensusRule = {
  name: "modifier-cap",
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        (file.area === "src" || file.area === "extensions") &&
        file.source !== undefined &&
        !file.rel.startsWith("src/i18n/"),
    ),
  floors: (corpus) => [
    ...(corpus.length > 100 ? [] : ["fewer than 100 modules were read"]),
    ...(corpus.some(
      (file) =>
        file.rel === "src/lib/keymap.ts" &&
        file.text.includes('"shell.shortcuts.ctrl"'),
    )
      ? []
      : ["src/lib/keymap.ts no longer takes the Ctrl cap from the catalog"]),
  ],
  visit: sitesIn,
  advice:
    "Draw shortcut caps from src/lib/keymap.ts (bindingCaps, shortcutTip), " +
    "which takes the Ctrl cap from the catalog key shell.shortcuts.ctrl.",
};
