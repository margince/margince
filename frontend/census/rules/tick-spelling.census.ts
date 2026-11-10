// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Tick gate: `Checkbox` and `Radio` in field.tsx are the product's only tick
// controls. A tick is four decisions: the box's size, its accent, its focus
// ring, and the words as the other half of the click target.
//
// Three shapes are a second spelling of one:
//
//   a tick role (`checkbox`, `menuitemcheckbox`, `radio`, `menuitemradio`) on
//   a non-input claims a control the browser is not driving;
//   `aria-checked` on anything but an input makes the same claim; `Switch` is
//   a different control that announces `role="switch"`;
//   a bare `<input type="checkbox">` or `type="radio"` outside field.tsx
//   decides the size and the gap again.
//
// Out of scope: `Switch`, a listbox option's mark, an `aria-pressed` toggle,
// and `SegmentedControl`. Stories and tests are out of the census:
// `interaction.stories.tsx` renders bare native controls to show `accent-color`.

import { join } from "node:path";
import ts from "typescript";
import { parseSource, scriptKindFor } from "../../scripts/lib/source-tree";
import { type CensusFile, type CensusRule, srcDir } from "../tree";

/** The roles a native input already carries. */
export const TICK_ROLES: ReadonlySet<string> = new Set([
  "checkbox",
  "menuitemcheckbox",
  "radio",
  "menuitemradio",
]);

/** The input types `Toggle` wraps. */
export const TICK_TYPES: ReadonlySet<string> = new Set(["checkbox", "radio"]);

// The two owners, each exempt from the arm it owns and from no other. Named by
// path, because a parser cannot recognise "the file that renders the atom".
export const FIELD = join(srcDir, "design-system", "field.tsx");
export const SWITCH = join(srcDir, "design-system", "switch.tsx");

/** Whether a file is a story or a test, which the census leaves out. */
export function isStoryOrTest(path: string): boolean {
  return /\.(stories|test)\.[cm]?[jt]sx?$/.test(path);
}

function attributeName(attribute: ts.JsxAttributeLike): string | null {
  return ts.isJsxAttribute(attribute) && ts.isIdentifier(attribute.name)
    ? attribute.name.text
    : null;
}

/** A JSX attribute's value when it is a plain string, with or without braces. */
function stringValue(attribute: ts.JsxAttributeLike): string | null {
  if (!ts.isJsxAttribute(attribute)) return null;
  const value = attribute.initializer;
  if (value && ts.isStringLiteral(value)) return value.text;
  if (
    value &&
    ts.isJsxExpression(value) &&
    value.expression &&
    ts.isStringLiteral(value.expression)
  ) {
    return value.expression.text;
  }
  return null;
}

/**
 * One report line per hand-rolled tick in the source, with the author's line.
 * A file that spells none of the three shapes is not parsed.
 */
export function findTickSpellings(path: string, text: string): string[] {
  if (!/role|aria-checked|checkbox|radio/.test(text)) return [];
  const source = parseSource(path, text);
  const found: string[] = [];
  // A non-TSX file is read again as TSX: JSX the first parse cannot see
  // yields no nodes and would pass unseen (native-controls.census.ts).
  if (scriptKindFor(path) !== ts.ScriptKind.TSX) {
    for (const hit of findTickSpellings(`${path}x`, text)) {
      if (!found.includes(hit)) found.push(hit);
    }
  }
  const at = (node: ts.Node) =>
    source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1;

  const visit = (node: ts.Node): void => {
    if (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) {
      const tag = ts.isIdentifier(node.tagName) ? node.tagName.text : "";
      const isInput = tag === "input";
      for (const attribute of node.attributes.properties) {
        const name = attributeName(attribute);
        if (name === "role" && !isInput) {
          const role = stringValue(attribute);
          if (role && TICK_ROLES.has(role)) {
            found.push(`${at(attribute)}: role="${role}" on <${tag}>`);
          }
        }
        if (name === "aria-checked" && !isInput && path !== SWITCH) {
          found.push(`${at(attribute)}: aria-checked on <${tag}>`);
        }
        if (name === "type" && isInput && path !== FIELD) {
          const type = stringValue(attribute);
          if (type && TICK_TYPES.has(type)) {
            found.push(`${at(attribute)}: <input type="${type}">`);
          }
        }
      }
    }
    ts.forEachChild(node, visit);
  };
  ts.forEachChild(source, visit);
  return [...new Set(found)];
}

const judged = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  file.source !== undefined &&
  !isStoryOrTest(file.path);

export const rule: CensusRule = {
  name: "tick-spelling",
  corpus: (tree) => tree.files.filter(judged),
  // Three floors: a walk pointed at the wrong tree, one that missed the
  // screens, and one that stopped at src/.
  floors: (corpus) => [
    ...(corpus.length > 100 ? [] : ["fewer than 100 files were read"]),
    ...(corpus.some((file) => file.rel.startsWith("src/features/"))
      ? []
      : ["the census covered no screen under src/features/"]),
    ...(corpus.some((file) => file.area === "extensions")
      ? []
      : ["the census reached no extension frontend layer"]),
  ],
  visit: (file) =>
    findTickSpellings(file.path, file.text).map((hit) => `${file.rel}:${hit}`),
  advice:
    "Use Checkbox or Radio from src/design-system/atoms.tsx (ChoiceList for a " +
    "whole question); see src/design-system/README.md. A setting that writes " +
    "when you flip it is a Switch, and a verb left on is a pressed Button.",
};
