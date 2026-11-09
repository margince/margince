// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Two promises of Button that only the whole tree can hold.
//
// The bare variant is the `.iconbtn` control: every rule for one has a twin for
// the other, with the same declarations in the same sheet.
//
// A glyph inside a control takes no size of its own. lucide's `size` writes
// attributes that every author rule outranks, so it is a false claim. An inline
// style wins, and draws the glyph at a size the control did not choose.

import ts from "typescript";
import { rulesOfText } from "../../scripts/lib/css-rules";
import type { CensusFile, CensusRule } from "../tree";

// The pairs written today. Pinned, so a pair deleted on both sides fails.
export const PAIRS = 28;

/** `.btn-bare` alone ties with `.btn`, so every mention is `.btn.btn-bare`. */
export function namesLoneBare(selector: string): boolean {
  return /(?<!(?:^|[^\w-])\.btn)\.btn-bare(?![\w-])/.test(selector);
}

/** The same selector spelled for the other of the pair, if it names one. */
export function bareTwinOf(selector: string): string | undefined {
  const iconbtn = /\.iconbtn(?![\w-])/g;
  const bare = /\.btn\.btn-bare(?![\w-])/g;
  if (selector.match(iconbtn)) {
    return selector.replace(iconbtn, ".btn.btn-bare");
  }
  if (selector.match(bare)) {
    return selector.replace(bare, ".iconbtn");
  }
  return undefined;
}

type SheetRule = { rel: string; selector: string; body: string };

const sheetRules = (sheets: readonly CensusFile[]): SheetRule[] =>
  sheets.flatMap((file) =>
    rulesOfText(file.path, file.text).map(({ selector, body }) => ({
      rel: file.rel,
      selector,
      body,
    })),
  );

/** The twin pairs, and every rule whose twin is missing or misspelled. */
export function bareTwins(sheets: readonly CensusFile[]): {
  pairs: number;
  findings: string[];
} {
  const all = sheetRules(sheets);
  const key = (rel: string, selector: string, body: string) =>
    `${rel}|${selector}|${body.trim()}`;
  const written = new Set(
    all.map((one) => key(one.rel, one.selector, one.body)),
  );
  const paired = all.flatMap((one) => {
    const partner = bareTwinOf(one.selector);
    return partner === undefined
      ? []
      : [{ one, partner: key(one.rel, partner, one.body) }];
  });
  const where = (one: SheetRule) => `${one.rel}: ${one.selector}`;
  return {
    pairs: paired.length,
    findings: [
      ...paired
        .filter(({ partner }) => !written.has(partner))
        .map(({ one }) => `${where(one)} has no .iconbtn / .btn.btn-bare twin`),
      ...all
        .filter((one) => namesLoneBare(one.selector))
        .map((one) => `${where(one)} names .btn-bare without .btn`),
    ],
  };
}

const CONTROL_TAGS = new Set(["Button", "IconAction"]);
// The classes the control sheets key on, so an `<a className="btn">` is judged
// as the control it is drawn as.
const CONTROL_CLASS = /\b(btn|iconbtn|link-button)\b/;

const openingOf = (node: ts.Node) =>
  ts.isJsxElement(node)
    ? node.openingElement
    : ts.isJsxSelfClosingElement(node)
      ? node
      : undefined;

function isControl(node: ts.Node): boolean {
  const open = openingOf(node);
  if (open === undefined) return false;
  if (CONTROL_TAGS.has(open.tagName.getText())) return true;
  return open.attributes.properties.some(
    (attribute) =>
      ts.isJsxAttribute(attribute) &&
      attribute.name.getText() === "className" &&
      attribute.initializer !== undefined &&
      CONTROL_CLASS.test(attribute.initializer.getText()),
  );
}

// The `size` prop that states a number, and the inline width or height.
function sizesItself(attribute: ts.JsxAttributeLike): boolean {
  if (!ts.isJsxAttribute(attribute)) return false;
  const name = attribute.name.getText();
  const text = attribute.getText();
  if (name === "size") return /^size=\{?\d+\}?$/.test(text);
  if (name === "style") {
    return /\b(width|height|inlineSize|blockSize)\s*:/.test(text);
  }
  return false;
}

/** Every glyph inside a control that names a size of its own. */
export function sizedGlyphs(rel: string, source: ts.SourceFile): string[] {
  const found: string[] = [];
  const walk = (node: ts.Node, inControl: boolean) => {
    const inside = inControl || isControl(node);
    const open = openingOf(node);
    // The control's own props are its business; what it wraps is judged.
    if (
      open !== undefined &&
      inside &&
      !CONTROL_TAGS.has(open.tagName.getText())
    ) {
      for (const attribute of open.attributes.properties) {
        if (!sizesItself(attribute)) continue;
        const { line } = source.getLineAndCharacterOfPosition(
          attribute.getStart(),
        );
        found.push(
          `${rel}:${line + 1}: <${open.tagName.getText()} ${attribute.getText()}>`,
        );
      }
    }
    node.forEachChild((child) => walk(child, inside));
  };
  walk(source, false);
  return found;
}

const sheetsIn = (corpus: readonly CensusFile[]) =>
  corpus.filter((file) => file.rel.endsWith(".css"));

export const rule: CensusRule = {
  name: "button",
  // A unit's screen draws this tier's controls, so the extension layers count.
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        (file.area === "src" || file.area === "extensions") &&
        /\.(tsx|css)$/.test(file.rel),
    ),
  floors: (corpus) => {
    const pairs = bareTwins(sheetsIn(corpus)).pairs;
    const tsx = corpus.filter((file) => file.rel.endsWith(".tsx"));
    return [
      ...(tsx.length > 100 ? [] : ["fewer than 100 .tsx files were read"]),
      ...(tsx.some((file) => file.area === "extensions")
        ? []
        : ["the census reached no extension frontend layer"]),
      ...(pairs === PAIRS
        ? []
        : [
            `${pairs} .iconbtn / .btn.btn-bare rules are paired and the pin is ${PAIRS}`,
          ]),
    ];
  },
  across: (corpus) => [
    ...bareTwins(sheetsIn(corpus)).findings,
    ...corpus.flatMap((file) =>
      file.source && file.rel.endsWith(".tsx")
        ? sizedGlyphs(file.rel, file.source)
        : [],
    ),
  ],
  advice:
    "A rule for .iconbtn needs its .btn.btn-bare twin with the same body in " +
    "the same sheet, and `.btn-bare` is always spelled `.btn.btn-bare`. A " +
    "glyph inside a Button, IconAction or btn-classed element takes no size " +
    "prop and no inline width or height: the control sizes it.",
};
