// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A dialog's field rows have an owner for the space between them: a stack's
// gap, and not a margin on one row. The walk follows every dialog, wrappers
// included, into the components it renders; scripts/lib/dialogrows.ts holds it.

import type ts from "typescript";
import {
  FIELD_ROWS,
  labelledIn,
  NOT_ROWS,
} from "../../scripts/lib/dialogfields";
import {
  defOf,
  dialogSet,
  elementsIn,
  keyOf,
  ownersIn,
  primitiveKey,
  textCensus,
  whereOf,
} from "../../scripts/lib/dialoglayout";
import { dialogsIn, hints, rowsAcross } from "../../scripts/lib/dialogrows";
import type { CensusFile, CensusRule, CensusTree } from "../tree";

// Stories are in: a reader copies them. Tests are out: their Modals are fixtures.
const isMarkup = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  /\.(tsx|jsx)$/.test(file.path) &&
  !/\.test\.(tsx|jsx)$/.test(file.path);

const isSheet = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  file.path.endsWith(".css");

const sourcesOf = (corpus: readonly CensusFile[]): ts.SourceFile[] =>
  corpus.flatMap((file) => file.source ?? []);

// The walk is the costly part, and both floors and findings read it.
const walks = new WeakMap<CensusTree, ReturnType<typeof walk>>();

function walk(corpus: readonly CensusFile[], tree: CensusTree) {
  const sources = sourcesOf(corpus);
  const dialogs = dialogSet(sources);
  const owners = ownersIn(tree.files.filter(isSheet).map((file) => file.text));
  return {
    sources,
    dialogs,
    rows: rowsAcross(sources, owners, dialogs),
    spelled: textCensus(corpus.map((file) => file.text)).dialogs,
  };
}

function walked(corpus: readonly CensusFile[], tree: CensusTree) {
  const known = walks.get(tree);
  if (known) return known;
  const made = walk(corpus, tree);
  walks.set(tree, made);
  return made;
}

// Every component a dialog renders is judged, or is a dialog or a field row.
function unjudged(found: ReturnType<typeof walk>): string[] {
  const { keys } = found.dialogs;
  const fields = new Set(
    [...FIELD_ROWS].map(([name, file]) => primitiveKey(file, name)),
  );
  const shown = found.sources.flatMap((source) =>
    dialogsIn(source, keys).flatMap((dialog) =>
      elementsIn(dialog).flatMap((element) => defOf(element) ?? []),
    ),
  );
  const skipped = (def: ts.Node) =>
    keys.has(keyOf(def)) ||
    fields.has(keyOf(def)) ||
    found.rows.judged.has(def);
  return [...new Set(shown)]
    .filter((def) => !skipped(def))
    .map((def) => `${whereOf(def)} renders inside a dialog and was not judged`);
}

// The design-system controls that draw their own label are the ones the walk
// knows as field rows, or knows are not rows.
function unlisted(corpus: readonly CensusFile[]): string[] {
  const found = new Set(
    sourcesOf(
      corpus.filter(
        (file) =>
          file.path.includes("/design-system/") &&
          !file.path.endsWith(".stories.tsx"),
      ),
    ).flatMap((source) => labelledIn(source)),
  );
  const listed = new Set([...FIELD_ROWS.keys(), ...NOT_ROWS.keys()]);
  return [
    ...[...found]
      .filter((name) => !listed.has(name))
      .map(
        (name) =>
          `${name} draws its own label and is in neither FIELD_ROWS nor NOT_ROWS`,
      ),
    ...[...listed]
      .filter((name) => !found.has(name))
      .map(
        (name) =>
          `${name} is listed in scripts/lib/dialogfields.ts and draws no label`,
      ),
  ];
}

export const rule: CensusRule = {
  name: "dialogrows",
  corpus: (tree) => tree.files.filter(isMarkup),
  floors: (corpus, tree) => {
    const found = walked(corpus, tree);
    const cut = new Set([
      found.dialogs.modal,
      primitiveKey("confirmmodal", "ConfirmModal"),
    ]);
    const shortWalk = found.sources.reduce(
      (n, source) => n + dialogsIn(source, cut).length,
      0,
    );
    return [
      ...(found.rows.dialogs === found.spelled
        ? []
        : [
            `the walk reached ${found.rows.dialogs} dialogs and the text spells ${found.spelled}`,
          ]),
      ...(found.dialogs.keys.size > 1 ? [] : ["no dialog wrapper was found"]),
      ...(found.rows.rows.length > 0 ? [] : ["no field row was found"]),
      ...(found.rows.depth > 1
        ? []
        : ["the walk followed no component into a second file"]),
      // With only Modal and ConfirmModal the walk counts short of the text,
      // so the wrappers it derives are doing work.
      ...(shortWalk < found.spelled
        ? []
        : ["Modal and ConfirmModal alone reach every dialog the text spells"]),
    ];
  },
  across: (corpus, tree) => {
    const found = walked(corpus, tree);
    return [
      ...unjudged(found),
      ...unlisted(corpus),
      ...found.rows.rows
        .filter((row) => row.verdict !== "spaced")
        .map(
          (row) =>
            `${row.where} ${row.verdict}${row.by ? ` by ${row.by}` : ""}`,
        ),
    ];
  },
  advice: `flush: ${hints.flush}; doubled: ${hints.doubled}; unplaced: ${hints.unplaced}`,
};
