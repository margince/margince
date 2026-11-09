// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { ownersIn, srcDir } from "../../scripts/lib/dialoglayout";
import { plantedFindings } from "../tree";
import { rule, titlesIn } from "./dialogtitle.census";

// The sheet that declares the house owners, read by name rather than walked.
const owners = ownersIn([
  readFileSync(join(srcDir, "design-system", "atoms.css"), "utf8"),
]);

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

describe("a dialog's Heading title has an owner for the space under it", () => {
  it("derives the owning containers from the stylesheets", () => {
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
});

describe("the dialogtitle rule", () => {
  it("reports a planted orphan title through its own corpus and across", () => {
    const text = `${PRELUDE}\nexport const A = () => (${modal(`${T}<p />`)});\n`;
    const findings = plantedFindings(rule, [{ path: AT, area: "src", text }]);
    expect(findings).toContain(
      "design-system/planted.tsx:3: a dialog title with no owner for the space under it",
    );
  });
});
