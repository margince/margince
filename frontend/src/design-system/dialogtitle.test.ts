// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join, relative } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  attr,
  attrs,
  childrenSlot,
  classes,
  defOf,
  dialogSet,
  elementsIn,
  enclosing,
  idParts,
  keyOf,
  markupFiles,
  type Owners,
  ownersIn,
  primitiveKey,
  sheetTexts,
  srcDir,
  tag,
  textCensus,
} from "../../scripts/lib/dialoglayout";
import { parseSource } from "../../scripts/lib/source-tree";

const EXCEPTIONS = [
  {
    file: "design-system/modal.stories.tsx",
    classes: "t-h2",
    reason: "a title and its subtitle pair tight inside the band",
  },
];
type Title = { where: string; classes: string[]; owner: string | null };

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

function titlesIn(path: string, text: string, owners: Owners) {
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
    (n) => ts.isJsxElement(n) && is(dialogSet().modal)(n),
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

const HEADING = primitiveKey("heading", "Heading");
const owners = ownersIn(sheetTexts());
const texts = markupFiles().map((f) => ({ f, text: readFileSync(f, "utf8") }));
const corpus = texts.map(({ f, text }) => titlesIn(f, text, owners));
const titles = corpus.flatMap((c) => c.titles);
const matches = (e: (typeof EXCEPTIONS)[number], t: Title) =>
  t.where.startsWith(`${e.file}:`) && t.classes.join(" ") === e.classes;
const exempt = (t: Title) => EXCEPTIONS.some((e) => matches(e, t));

const AT = join(srcDir, "design-system", "planted.tsx");
const PRELUDE = `import { Modal, Modal as Dialog } from "./modal";
import { Heading, Heading as H } from "./heading";`;
const BODY = `function Body({ titleId }) { return <div>${h("id={titleId}")}</div>; }
function Band({ children }) { return <div className="drawer-head">{children}</div>; }`;
function h(attributes: string) {
  return `<Heading size="large" ${attributes}>A</Heading>`;
}
const planted = (jsx: string, own = owners) => {
  const text = `${PRELUDE}\nconst A = () => (${jsx});\n${BODY}`;
  const { titles: found, unresolved } = titlesIn(AT, text, own);
  if (unresolved.length > 0) return "unresolved";
  return found.map((t) => t.owner ?? "orphan").join(" ") || "no title";
};
const modal = (inner: string, by = '"t"') =>
  `<Modal labelledBy=${by}>${inner}</Modal>`;
const T = h('id="t"');
type Case = { jsx: string; verdict: string; sheet: string };
const TEMPLATE = "className={`t-h2 $" + "{x} modal-title`}";
const MULTILINE =
  '<Modal\n  labelledBy="t"\n>\n  <Heading\n    size="large"\n    id="t"\n    className="modal-title"\n  >\n    A\n  </Heading>\n</Modal>';
const ROW = ".frow { display: flex; gap: var(--space-3) }";
const COLUMN =
  ".x { display: flex; flex-direction: column; gap: var(--space-3) }";

describe("a dialog's Heading title has an owner for the space under it (a <p> title is out of scope)", () => {
  it("walks every Modal in the tree and resolves each to its title", () => {
    const modals = corpus.reduce((n, c) => n + c.modals, 0);
    const { modals: spelled } = textCensus(texts.map((t) => t.text));
    expect(spelled).toBeGreaterThan(0);
    expect(modals).toBe(spelled);
    expect(corpus.flatMap((c) => c.unresolved)).toEqual([]);
  });

  it("derives the owning containers from the stylesheets", () => {
    expect(owners.gap).toContain("form-stack");
    expect(owners.band).toContain("drawer-head");
    expect(owners.band).toContain("modal-title");
    const own = ownersIn([
      ".row { display: flex; gap: var(--space-3) }",
      ".grid { display: grid; gap: var(--space-3) }",
      ".band { padding: 0 0 var(--space-4) }",
      ".a:has(> .inner) { padding-bottom: var(--space-3) }",
      ".none { margin-bottom: 0 !important; padding-bottom: unset }",
      ".short { margin: 0 auto !important }",
      "@media (min-width: 1px) { .wide { display: grid; gap: var(--space-3) } }",
    ]);
    expect([[...own.gap], [...own.band]]).toEqual([["grid"], ["band", "a"]]);
  });

  it.each`
    spec                                                 | jsx                                                             | verdict            | sheet
    ${"catches a title with no class"}                   | ${modal(`${T}<p />`)}                                           | ${"orphan"}        | ${""}
    ${"passes a title wearing modal-title"}              | ${modal(h('id="t" className="modal-title"'))}                   | ${"modal-title"}   | ${""}
    ${"passes a title opening a .form-stack"}            | ${modal(`<form className="form-stack">${T}<p /></form>`)}       | ${"gap container"} | ${""}
    ${"catches a title alone in a column gap container"} | ${modal(`<div className="x">${T}</div><p />`)}                  | ${"orphan"}        | ${COLUMN}
    ${"catches a title in a flex row with a gap"}        | ${modal(`<div className="frow">${T}<button /></div><p />`)}     | ${"orphan"}        | ${ROW}
    ${"catches a title in a bare wrapper"}               | ${modal(`<div>${T}</div>`)}                                     | ${"orphan"}        | ${""}
    ${"reads the band a component draws around a title"} | ${modal(`<Band>${T}</Band><p />`)}                              | ${"head band"}     | ${""}
    ${"catches a band with content under the title"}     | ${modal(`<div className="band">${T}<p /></div>`)}               | ${"orphan"}        | ${".band { padding-bottom: var(--space-3) }"}
    ${"catches a band that :has() paints on its parent"} | ${modal(`<div className="x">${T}</div><p />`)}                  | ${"orphan"}        | ${".a:has(> .x) { padding-bottom: var(--space-3) }"}
    ${"treats only a <p> as a title outside scope"}      | ${modal('<p id="t">A</p>')}                                     | ${"no title"}      | ${""}
    ${"calls a <div id> title carrier unresolved"}       | ${modal(`<div id="t">${h("")}</div><p />`)}                     | ${"unresolved"}    | ${""}
    ${"calls a title it cannot reach unresolved"}        | ${modal("<Other />")}                                           | ${"unresolved"}    | ${""}
    ${"reads Modal imported under another name"}         | ${`<Dialog labelledBy="t">${T}<p /></Dialog>`}                  | ${"orphan"}        | ${""}
    ${"reads Heading imported under another name"}       | ${modal('<H size="large" id="t">A</H><p />')}                   | ${"orphan"}        | ${""}
    ${"reads an opening tag over several lines"}         | ${MULTILINE}                                                    | ${"modal-title"}   | ${""}
    ${"reads a className built by a template"}           | ${modal(h(`id={t} ${TEMPLATE}`), "{t}")}                        | ${"modal-title"}   | ${""}
    ${"reads a className built by cx()"}                 | ${modal(h('id={t} className={cx("a", "modal-title")}'), "{t}")} | ${"modal-title"}   | ${""}
    ${"follows an id prop one component away"}           | ${modal("<Body titleId={t} />", "{t}")}                         | ${"orphan"}        | ${""}
  `("$spec", ({ jsx, verdict, sheet }: Case) => {
    expect(planted(jsx, sheet ? ownersIn([sheet]) : owners)).toBe(verdict);
  });

  it("has an owner for every title in the tree", () => {
    const orphans = titles.filter((t) => t.owner === null && !exempt(t));
    expect(
      orphans.map((t) => t.where),
      "give the title modal-title, or open a gap container with it",
    ).toEqual([]);
  });

  it("keeps each exception on the one title that still needs it", () => {
    for (const e of EXCEPTIONS) {
      const still = titles.filter((t) => exempt(t) && t.owner === null);
      const here = still.filter((t) => matches(e, t));
      expect(here, e.reason).toHaveLength(1);
    }
  });
});
