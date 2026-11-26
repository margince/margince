// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A refused write inside a dialog or a drawer has one spelling: DialogRefusal.
// A danger outcome Callout or a WriteRefused there draws the same card, but it
// stays under the pinned foot where the reader never sees it arrive. The walk
// starts at every dialog and follows each screen component it renders, into
// other files too; a design-system component owns its own shape and is not
// entered.

import ts from "typescript";
import {
  attr,
  defOf,
  dialogSet,
  elementsIn,
  keyOf,
  primitiveKey,
  srcDir,
  textCensus,
  whereOf,
} from "../../scripts/lib/dialoglayout";
import { dialogsIn } from "../../scripts/lib/dialogrows";
import type { CensusFile, CensusRule, CensusTree } from "../tree";

const CALLOUT = primitiveKey("callout", "Callout");
const WRITE_REFUSED = primitiveKey("querystates", "WriteRefused");
const designSystem = `${srcDir}/design-system/`;

// Stories are in: a reader copies them. Tests are out: their Modals are fixtures.
const isMarkup = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  /\.(tsx|jsx)$/.test(file.path) &&
  !/\.test\.(tsx|jsx)$/.test(file.path);

function literal(element: ts.Node, name: string): string | undefined {
  const value = attr(element, name)?.initializer;
  const said = value && ts.isJsxExpression(value) ? value.expression : value;
  return said && ts.isStringLiteralLike(said) ? said.text : undefined;
}

/** What a refusal spelled by hand is drawn as, or undefined for anything else. */
export function handSpelled(element: ts.Node): string | undefined {
  const def = defOf(element);
  const key = def ? keyOf(def) : undefined;
  if (key === WRITE_REFUSED) return "WriteRefused";
  const refusal =
    key === CALLOUT &&
    literal(element, "tone") === "danger" &&
    literal(element, "kind") === "outcome";
  return refusal ? "a danger outcome Callout" : undefined;
}

type Walk = {
  dialogs: number;
  spelled: number;
  entered: number;
  found: Map<string, string>;
};

// Every element a dialog draws: its own JSX, then each screen component's, once.
function walk(corpus: readonly CensusFile[]): Walk {
  const sources = corpus.flatMap((file) => file.source ?? []);
  const { keys } = dialogSet(sources);
  const dialogs = sources.flatMap((source) => dialogsIn(source, keys));
  const entered = new Set<ts.Node>();
  const found = new Map<string, string>();
  const queue: ts.Node[] = [...dialogs];
  for (let root = queue.pop(); root; root = queue.pop()) {
    for (const element of elementsIn(root)) {
      const spelled = handSpelled(element);
      if (spelled) found.set(whereOf(element), spelled);
      const def = defOf(element);
      const screen =
        def && !def.getSourceFile().fileName.startsWith(designSystem);
      if (def && screen && !entered.has(def)) {
        entered.add(def);
        queue.push(def);
      }
    }
  }
  const files = new Set([...entered].map((def) => def.getSourceFile()));
  return {
    dialogs: dialogs.length,
    spelled: textCensus(corpus.map((file) => file.text)).dialogs,
    entered: files.size,
    found,
  };
}

const walks = new WeakMap<CensusTree, Walk>();

function walked(corpus: readonly CensusFile[], tree: CensusTree): Walk {
  const known = walks.get(tree);
  if (known) return known;
  const made = walk(corpus);
  walks.set(tree, made);
  return made;
}

export const rule: CensusRule = {
  name: "dialog-refusal",
  corpus: (tree) => tree.files.filter(isMarkup),
  floors: (corpus, tree) => {
    const { dialogs, spelled, entered } = walked(corpus, tree);
    return [
      ...(dialogs === spelled
        ? []
        : [`the walk reached ${dialogs} dialogs and the text spells ${spelled}`]),
      ...(entered > 1
        ? []
        : ["the walk followed no component into a second file"]),
    ];
  },
  across: (corpus, tree) =>
    [...walked(corpus, tree).found].map(
      ([where, spelled]) =>
        `${where}: ${spelled} says a refusal inside a dialog`,
    ),
  advice:
    "A refused write inside a dialog or a drawer is DialogRefusal from " +
    "design-system/panelnotices.tsx: the same card, scrolled into view when " +
    "it lands under the dialog's pinned foot.",
};
