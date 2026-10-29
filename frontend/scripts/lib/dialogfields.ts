// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Which controls stand as a dialog's form rows, and which draw their own label.

import ts from "typescript";
import type { Given } from "../../src/testing/classnames";
import {
  attr,
  declarations,
  definition,
  defOf,
  elementsIn,
  enclosing,
  keyOf,
  primitiveKey,
  rootsOf,
  tag,
  variantsOf,
} from "./dialoglayout";

export const FIELD_ROWS = new Map([
  ["Field", "atoms"],
  ["Checkbox", "atoms"],
  ["Radio", "atoms"],
  ["ChoiceList", "choicelist"],
  ["SetPicker", "setpicker"],
]);
// Design-system controls that draw their own label and are not form rows.
export const NOT_ROWS = new Map([
  ["InlineText", "edits a value in place, its label hidden"],
  ["ChipValueList", "picks a list filter's values"],
  ["Toolbar", "a list's search box, a label around its input in the toolbar"],
]);
const rowKeys = [...FIELD_ROWS].map(([name, file]) => primitiveKey(file, name));
export const keyIs = (n: ts.Node, key: string) => {
  const def = defOf(n);
  return !!def && keyOf(def) === key;
};
// Only a label hidden in every render: `labelHidden={x}` shows it in one.
const inPlace = (n: ts.Node) => {
  const hidden = attr(n, "labelHidden");
  const e = hidden?.initializer;
  return (
    !!hidden &&
    (!e || (ts.isJsxExpression(e) && e.expression?.getText() === "true"))
  );
};
// A field whose label is hidden edits a value in place among its text,
// rather than standing as a row of a form.
export const isPrimitiveField = (n: ts.Node) =>
  rowKeys.some((k) => keyIs(n, k)) && !inPlace(n);
export const isField = (n: ts.Node, given?: Given) =>
  isPrimitiveField(n) || variantsOf(n, given).flat().includes("form-row");
// A root that is already a row counts as one where the component is used,
// so it is not followed.
function drawsLabel(def: ts.Node, hops = 2): boolean {
  return rootsOf(def).some((r) => {
    if (tag(r) === "label") return true;
    const inside = elementsIn(r).filter((e) => enclosing(e) === r);
    if (inside.some((e) => ["label", "legend"].includes(tag(e)))) return true;
    const inner = defOf(r);
    if (!inner || isField(r)) return false;
    return hops > 0 && drawsLabel(inner, hops - 1);
  });
}
const exported = (def: ts.Node) =>
  (ts.isFunctionDeclaration(def) || ts.isVariableDeclaration(def)) &&
  (ts.getCombinedModifierFlags(def) & ts.ModifierFlags.Export) !== 0;
export const labelledIn = (source: ts.SourceFile) =>
  [...declarations(source)]
    .filter(([name, def]) => /^[A-Z]/.test(name) && exported(def))
    .filter(([name]) => {
      const drawn = definition(source, name);
      return !!drawn && drawsLabel(drawn);
    })
    .map(([name]) => name);
