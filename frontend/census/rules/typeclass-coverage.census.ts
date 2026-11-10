// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A type class a component asks for must exist. `base.css` owns the type
// scale, and a component asks for a step by name (`t-caption`, `t-label`). An
// undeclared name gets no warning: the element inherits, and a card's figure
// renders at body size.
//
// Both sides come from the tree: every `.tsx` under src/ for the ask, every
// `.css` for the declaration. Not caught: the same defect in a class outside
// the type scale, which needs a census of every sheet's selectors.

import type { CensusFile, CensusRule } from "../tree";

const TYPE_CLASS = /^t-[a-z0-9-]+$/;

// Every spelling a className takes: a plain string, a string in braces with
// either quote, and a template literal. A `${…}` hole splits like whitespace,
// so a computed half adds no token.
const CLASS_NAME =
  /className=(?:"([^"]*)"|\{\s*"([^"]*)"\s*\}|\{\s*'([^']*)'\s*\}|\{`([^`]*)`\})/g;

/** Every `t-*` class one module asks for, with the line that asks. */
export function asked(rel: string, text: string): [string, string][] {
  return [...text.matchAll(CLASS_NAME)].flatMap((match) => {
    const line = text.slice(0, match.index).split("\n").length;
    const spelled = match[1] ?? match[2] ?? match[3] ?? match[4] ?? "";
    return spelled
      .split(/[\s{}$`]+/)
      .filter((token) => TYPE_CLASS.test(token))
      .map((token): [string, string] => [token, `${rel}:${line}`]);
  });
}

/** Every `t-*` class one stylesheet declares. */
export const declared = (text: string): string[] =>
  [...text.matchAll(/\.(t-[a-z0-9-]+)/g)].map((match) => match[1]);

const isSheet = (file: CensusFile) => file.rel.endsWith(".css");

function census(corpus: readonly CensusFile[]) {
  const sites = new Map<string, string[]>();
  for (const file of corpus.filter((one) => !isSheet(one))) {
    for (const [name, site] of asked(file.rel, file.text)) {
      sites.set(name, [...(sites.get(name) ?? []), site]);
    }
  }
  const have = new Set(
    corpus.filter(isSheet).flatMap((file) => declared(file.text)),
  );
  return { sites, have };
}

export const rule: CensusRule = {
  name: "typeclass-coverage",
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        file.area === "src" &&
        (file.rel.endsWith(".tsx") || file.rel.endsWith(".css")),
    ),
  // A regex that stopped matching would find nothing missing and pass.
  floors: (corpus) => {
    const { sites, have } = census(corpus);
    return [
      ...(sites.size >= 10 ? [] : ["fewer than 10 type classes are asked for"]),
      ...(have.size >= 10 ? [] : ["fewer than 10 type classes are declared"]),
    ];
  },
  across: (corpus) => {
    const { sites, have } = census(corpus);
    return [...sites]
      .filter(([name]) => !have.has(name))
      .map(([name, where]) => `${name}: ${where.slice(0, 3).join(", ")}`);
  },
  advice:
    "Ask only for a type class base.css declares. A quiet grey caption is " +
    "`t-caption`; declare a new step in the scale, never beside it.",
};
