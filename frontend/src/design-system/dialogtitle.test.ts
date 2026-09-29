// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { existsSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  classesOf,
  selectorList,
  splitTopLevel,
  subjectsOf,
} from "../../scripts/lib/css-rules";
import {
  extensionLayers,
  filesMatching,
  parseSource,
} from "../../scripts/lib/source-tree";
import { rulesIn } from "../testing/css";

const srcDir = join(dirname(fileURLToPath(import.meta.url)), "..");
const extensionsDir = join(srcDir, "..", "..", "extensions");
const underTree = (pattern: RegExp) =>
  filesMatching(srcDir, pattern).concat(
    extensionLayers(extensionsDir).flatMap((l) => filesMatching(l, pattern)),
  );

const EXCEPTIONS = [
  {
    file: "screens/onboarding-conversation/connect-dialog.tsx",
    classes: "ob-connect-dialog-title",
    reason: "the intro and body under it set their own margin-top",
  },
  {
    file: "design-system/modal.stories.tsx",
    classes: "t-h2",
    reason: "a title and its subtitle pair tight inside the band",
  },
];
const BOTTOM =
  /(?:^|[;\s])(?:(?:margin|padding)-(?:bottom|block-end)|border-bottom)\s*:([^;]+)/g;

type Owners = { gap: Set<string>; band: Set<string>; row: Set<string> };
type Title = { where: string; classes: string[]; owner: string | null };

const isZero = (v: string) =>
  /^(0[a-z%]*|none|auto|normal|unset|initial|inherit|revert(-layer)?)$/.test(
    v.replace(/!important/, "").trim(),
  );
const decl = (body: string, prop: string) =>
  body.match(new RegExp(`(?:^|[;\\s])${prop}\\s*:\\s*([^;]+)`))?.[1];
const display = (body: string, kind: string) =>
  new RegExp(`(^|[;\\s])display\\s*:\\s*(inline-)?${kind}\\b`).test(body);

function ownersIn(sheets: readonly string[]): Owners {
  const owners: Owners = { gap: new Set(), band: new Set(), row: new Set() };
  for (const { selector, body } of sheets.flatMap(rulesIn)) {
    const box = (p: string) => splitTopLevel(decl(body, p) ?? "", " \t\n");
    const rowGap = decl(body, "row-gap") ?? box("gap")[0] ?? "0";
    const direction = decl(body, "flex-direction") ?? decl(body, "flex-flow");
    const column = /^\s*column/.test(direction ?? "");
    const flex = display(body, "flex");
    const gap = (display(body, "grid") || (flex && column)) && !isZero(rowGap);
    const bottom = [
      ...[...body.matchAll(BOTTOM)].map((m) => m[1]),
      ...["margin-block", "padding-block"].map((p) => box(p).at(-1)),
      ...["margin", "padding"].map((p) => box(p)[box(p).length > 2 ? 2 : 0]),
    ].some((v) => v !== undefined && !isZero(v));
    const names = selectorList(selector)
      .flatMap(subjectsOf)
      .flatMap((subject) => [...classesOf(subject)]);
    for (const name of names) {
      if (gap) owners.gap.add(name);
      if (bottom) owners.band.add(name);
      if (flex && !column) owners.row.add(name);
    }
  }
  return owners;
}

const opening = (n: ts.Node) => {
  if (ts.isJsxElement(n)) return n.openingElement;
  return ts.isJsxSelfClosingElement(n) ? n : undefined;
};
const tag = (n: ts.Node) => opening(n)?.tagName.getText() ?? "";
const attrs = (n: ts.Node) => {
  const all: readonly ts.JsxAttributeLike[] =
    opening(n)?.attributes.properties ?? [];
  return all.filter(ts.isJsxAttribute);
};
const attr = (n: ts.Node, name: string) =>
  attrs(n).find((a) => a.name.getText() === name);
function descendants(root: ts.Node): ts.Node[] {
  const out: ts.Node[] = [];
  const visit = (n: ts.Node) => {
    out.push(n);
    ts.forEachChild(n, visit);
  };
  ts.forEachChild(root, visit);
  return out;
}
const elementsIn = (root: ts.Node) => descendants(root).filter(opening);
const enclosing = (n: ts.Node) => {
  for (let p = n.parent; p; p = p.parent) if (ts.isJsxElement(p)) return p;
  return undefined;
};
function keys(a: ts.JsxAttribute | undefined): string[] {
  const init = a?.initializer;
  const e = init && ts.isJsxExpression(init) ? init.expression : init;
  if (e && ts.isTemplateExpression(e)) {
    return e.templateSpans.map((span) => span.expression.getText());
  }
  return e ? [e.getText()] : [];
}
function classes(n: ts.Node): string[] {
  const init = attr(n, "className")?.initializer;
  return (init ? [init, ...descendants(init)] : [])
    .flatMap((x) =>
      ts.isStringLiteral(x) || ts.isTemplateLiteralToken(x)
        ? x.text.split(/\s+/)
        : [],
    )
    .filter(Boolean);
}
const imports = (source: ts.SourceFile) =>
  source.statements.filter(ts.isImportDeclaration);
function localNames(source: ts.SourceFile, name: string): Set<string> {
  const bound = imports(source).flatMap((d) => {
    const named = d.importClause?.namedBindings;
    if (!named || !ts.isNamedImports(named)) return [];
    const same = named.elements.filter(
      (e) => (e.propertyName ?? e.name).text === name,
    );
    return same.map((e) => e.name.text);
  });
  return new Set([name, ...bound]);
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

function componentSource(source: ts.SourceFile, name: string) {
  const from = imports(source).find((s) =>
    new RegExp(`\\b${name}\\b`).test(s.importClause?.getText() ?? ""),
  );
  if (!from) return source;
  const spec = from.moduleSpecifier.getText().slice(1, -1);
  const base = join(dirname(source.fileName), spec);
  const file = [`${base}.tsx`, `${base}.ts`, join(base, "index.tsx")].find(
    existsSync,
  );
  return file ? parseSource(file, readFileSync(file, "utf8")) : undefined;
}

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
    const around = parent === modal ? [] : classes(parent);
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
  const MODAL = localNames(source, "Modal");
  const HEADING = localNames(source, "Heading");
  const modals = elementsIn(source).filter(
    (n) => ts.isJsxElement(n) && MODAL.has(tag(n)),
  );
  for (const modal of modals) {
    const ids = keys(attr(modal, "labelledBy"));
    const labels = (a?: ts.JsxAttribute) =>
      keys(a).some((k) => ids.includes(k));
    const byId = (n: ts.Node) => labels(attr(n, "id"));
    const inside = elementsIn(modal);
    const first = inside.find((n) => enclosing(n) === modal);
    const found = inside.filter(
      (n) => HEADING.has(tag(n)) && (n === first || byId(n)),
    );
    const hop = (n: ts.Node) =>
      attrs(n)
        .filter((a) => a.name.getText() !== "id" && labels(a))
        .flatMap((a) => {
          const prop = a.name.getText();
          const target = componentSource(source, tag(n));
          if (!target) return [];
          const named = localNames(target, "Heading");
          return elementsIn(target).filter(
            (h) =>
              named.has(tag(h)) &&
              keys(attr(h, "id")).some(
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

const sheets = underTree(/\.css$/).map((f) => readFileSync(f, "utf8"));
const owners = ownersIn(sheets);
// Stories are in: a reader copies them. Tests are out: their Modals are fixtures.
const texts = underTree(/\.(tsx|jsx)$/)
  .filter((f) => !/\.test\.(tsx|jsx)$/.test(f))
  .map((f) => ({ f, text: readFileSync(f, "utf8") }));
const corpus = texts.map(({ f, text }) => titlesIn(f, text, owners));
const titles = corpus.flatMap((c) => c.titles);
const tagCensus = texts.reduce((n, { text }) => {
  const names = [
    "Modal",
    ...[...text.matchAll(/\bModal as (\w+)/g)].map((m) => m[1]),
  ];
  return (
    n + (text.match(new RegExp(`<(${names.join("|")})\\b`, "g"))?.length ?? 0)
  );
}, 0);
const matches = (e: (typeof EXCEPTIONS)[number], t: Title) =>
  t.where.startsWith(`${e.file}:`) && t.classes.join(" ") === e.classes;
const exempt = (t: Title) => EXCEPTIONS.some((e) => matches(e, t));

const PRELUDE =
  'import { Modal as Dialog } from "./modal";\nimport { Heading as H } from "./heading";';
const BODY = `function Body({ titleId }) { return <div>${h("id={titleId}")}</div>; }`;
function h(attributes: string) {
  return `<Heading size="large" ${attributes}>A</Heading>`;
}
const planted = (jsx: string, own = owners) => {
  const text = `${PRELUDE}\nconst A = () => (${jsx});\n${BODY}`;
  const { titles: found, unresolved } = titlesIn("planted.tsx", text, own);
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
    expect(tagCensus).toBeGreaterThan(0);
    expect(modals).toBe(tagCensus);
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
