// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A dialog's Heading title has an owner for the space under it: the head band,
// a gap container it opens, or `modal-title`. A <p> title is out of scope. The
// owning containers are derived from the stylesheets.

import { relative } from "node:path";
import ts from "typescript";
import {
  attr,
  attrs,
  childrenSlot,
  classes,
  defOf,
  elementsIn,
  enclosing,
  idParts,
  keyOf,
  type Owners,
  ownersIn,
  primitiveKey,
  srcDir,
  tag,
  textCensus,
} from "../../scripts/lib/dialoglayout";
import { parseSource } from "../../scripts/lib/source-tree";
import type { CensusFile, CensusRule, CensusTree } from "../tree";

const MODAL = primitiveKey("modal", "Modal");
const HEADING = primitiveKey("heading", "Heading");

const EXCEPTIONS = [
  {
    file: "design-system/modal.stories.tsx",
    classes: "t-h2",
    reason: "a title and its subtitle pair tight inside the band",
  },
];
export type Title = { where: string; classes: string[]; owner: string | null };

function hostClasses(n: ts.Node) {
  const host = childrenSlot(n);
  return host ? classes(host) : [];
}

const followed = (heading: ts.Node, parent: ts.Node) => {
  const kids = ts.isJsxElement(parent)
    ? parent.children.filter((c) =>
        ts.isJsxText(c)
          ? c.text.trim() !== ""
          : !ts.isJsxExpression(c) || c.expression,
      )
    : [];
  const mine = kids.findIndex(
    (c) => c.pos <= heading.pos && heading.end <= c.end,
  );
  return mine >= 0 && mine < kids.length - 1;
};

export function titlesIn(path: string, text: string, owners: Owners) {
  const source = parseSource(path, text);
  const titles: Title[] = [];
  const unresolved: string[] = [];
  const at = (n: ts.Node) => {
    const file = n.getSourceFile();
    const { line } = file.getLineAndCharacterOfPosition(n.getStart());
    return `${relative(srcDir, file.fileName)}:${line + 1}`;
  };
  const judge = (heading: ts.Node, modal: ts.Node) => {
    const parent = enclosing(heading) ?? modal;
    const around =
      parent === modal ? [] : [...classes(parent), ...hostClasses(parent)];
    const next = followed(heading, parent);
    const own = classes(heading).filter((c) => owners.band.has(c));
    let owner: string | null = null;
    const band = !next || around.some((c) => owners.row.has(c));
    if (band && around.some((c) => owners.band.has(c))) owner = "head band";
    if (next && around.some((c) => owners.gap.has(c))) owner = "gap container";
    if (own.length > 0)
      owner = own.includes("modal-title") ? "modal-title" : "own margin";
    titles.push({ where: at(heading), classes: classes(heading), owner });
  };
  const is = (key: string) => (n: ts.Node) => {
    const def = defOf(n);
    return !!def && keyOf(def) === key;
  };
  const isHeading = is(HEADING);
  const modals = elementsIn(source).filter(
    (n) => ts.isJsxElement(n) && is(MODAL)(n),
  );
  for (const modal of modals) {
    const ids = idParts(attr(modal, "labelledBy"));
    const labels = (a?: ts.JsxAttribute) =>
      idParts(a).some((k) => ids.includes(k));
    const byId = (n: ts.Node) => labels(attr(n, "id"));
    const inside = elementsIn(modal);
    const first = inside.find((n) => enclosing(n) === modal);
    const found = inside.filter(
      (n) => isHeading(n) && (n === first || byId(n)),
    );
    const hop = (n: ts.Node) =>
      attrs(n)
        .filter((a) => a.name.getText() !== "id" && labels(a))
        .flatMap((a) => {
          const prop = a.name.getText();
          const def = defOf(n);
          return (def ? elementsIn(def) : []).filter(
            (h) =>
              isHeading(h) &&
              idParts(attr(h, "id")).some(
                (k) => k === prop || k.endsWith(`.${prop}`),
              ),
          );
        });
    const components = inside.filter((n) => /^[A-Z]/.test(tag(n)));
    const own = found.length > 0 ? found : components.flatMap(hop);
    for (const heading of own) judge(heading, modal);
    const paragraph = inside.some((n) => tag(n) === "p" && byId(n));
    if (own.length === 0 && !paragraph) unresolved.push(at(modal));
  }
  return { modals: modals.length, titles, unresolved };
}

const matches = (e: (typeof EXCEPTIONS)[number], t: Title) =>
  t.where.startsWith(`${e.file}:`) && t.classes.join(" ") === e.classes;
const exempt = (t: Title) => EXCEPTIONS.some((e) => matches(e, t));

// Stories are in: a reader copies them. Tests are out: their Modals are fixtures.
const isMarkup = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  /\.(tsx|jsx)$/.test(file.path) &&
  !/\.test\.(tsx|jsx)$/.test(file.path);

const isSheet = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  file.path.endsWith(".css");

export const ownersOf = (tree: CensusTree): Owners =>
  ownersIn(tree.files.filter(isSheet).map((file) => file.text));

type Walk = {
  owners: Owners;
  modals: number;
  spelled: number;
  titles: Title[];
  unresolved: string[];
};

const walks = new WeakMap<CensusTree, Walk>();

function walked(corpus: readonly CensusFile[], tree: CensusTree): Walk {
  const known = walks.get(tree);
  if (known) return known;
  const owners = ownersOf(tree);
  const found = corpus.map((file) => titlesIn(file.path, file.text, owners));
  const made = {
    owners,
    modals: found.reduce((n, c) => n + c.modals, 0),
    spelled: textCensus(corpus.map((file) => file.text)).modals,
    titles: found.flatMap((c) => c.titles),
    unresolved: found.flatMap((c) => c.unresolved),
  };
  walks.set(tree, made);
  return made;
}

export const rule: CensusRule = {
  name: "dialogtitle",
  corpus: (tree) => tree.files.filter(isMarkup),
  floors: (corpus, tree) => {
    const { owners, modals, spelled } = walked(corpus, tree);
    return [
      ...(spelled > 0 ? [] : ["the text spells no Modal"]),
      ...(modals === spelled
        ? []
        : [`the walk reached ${modals} Modals and the text spells ${spelled}`]),
      ...["form-stack"]
        .filter((name) => !owners.gap.has(name))
        .map((name) => `.${name} is not read as a gap container`),
      ...["drawer-head", "modal-title"]
        .filter((name) => !owners.band.has(name))
        .map((name) => `.${name} is not read as a head band`),
    ];
  },
  across: (corpus, tree) => {
    const { titles, unresolved } = walked(corpus, tree);
    const stillExempt = (e: (typeof EXCEPTIONS)[number]) =>
      titles.filter((t) => matches(e, t) && t.owner === null).length;
    return [
      ...unresolved.map(
        (at) => `${at}: a Modal whose title the walk cannot resolve`,
      ),
      ...titles
        .filter((t) => t.owner === null && !exempt(t))
        .map(
          (t) =>
            `${t.where}: a dialog title with no owner for the space under it`,
        ),
      ...EXCEPTIONS.filter((e) => stillExempt(e) !== 1).map(
        (e) =>
          `${e.file} (${e.classes}) is excepted on ${stillExempt(e)} titles; an exception covers one title that still needs it`,
      ),
    ];
  },
  advice:
    "Give the title modal-title, or open a gap container with it. An " +
    "exception in census/rules/dialogtitle.census.ts covers one title, with " +
    "its reason.",
};
