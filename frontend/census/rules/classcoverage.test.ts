// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { e2eDir, plantedFindings, srcDir } from "../tree";
import {
  declaredIn,
  renderedIn,
  rule,
  selectorClassesIn,
} from "./classcoverage.census";

describe("what a className can be shown to produce", () => {
  const names = (markup: string) =>
    renderedIn(
      parseSource("probe.tsx", `const x = ${markup};`),
      "probe.tsx",
    ).map((one) => one.name);

  it("reads a plain list", () => {
    expect(names('<p className="a b">x</p>')).toEqual(["a", "b"]);
  });

  it("reads both branches of a conditional and not its question", () => {
    expect(names('<p className={on === "asc" ? "up" : "down"}>x</p>')).toEqual([
      "up",
      "down",
    ]);
  });

  it("reads a guard's class and not the guard", () => {
    expect(names('<p className={open && "row-open"}>x</p>')).toEqual([
      "row-open",
    ]);
  });

  it("reads every argument of an assembled list", () => {
    expect(names('<p className={cx("row", open && "on")}>x</p>')).toEqual([
      "row",
      "on",
    ]);
    expect(
      names('<p className={["row", "on"].filter(Boolean).join(" ")}>x</p>'),
    ).toEqual(["row", "on"]);
  });

  // The blind spot, stated as behaviour: the token touching the interpolation
  // is dropped rather than half-read.
  it("reads a template's whole tokens and drops the one it cannot finish", () => {
    // biome-ignore lint/suspicious/noTemplateCurlyInString: the placeholder is the subject: this string is the source text the reader parses
    expect(names("<p className={`card tone-${level}`}>x</p>")).toEqual([
      "card",
    ]);
    // biome-ignore lint/suspicious/noTemplateCurlyInString: the placeholder is the subject: this string is the source text the reader parses
    expect(names("<p className={`${prefix}-tail head`}>x</p>")).toEqual([
      "head",
    ]);
  });

  it("reads a concatenation as the text it joins", () => {
    expect(names('<p className={"card tone-" + level}>x</p>')).toEqual([
      "card",
    ]);
  });

  it("reads nothing out of an expression it cannot resolve", () => {
    expect(names("<p className={styles.row}>x</p>")).toEqual([]);
  });

  // The ordinary way a component with several conditional classes is written.
  it("follows a class list assembled into a local binding", () => {
    expect(
      names(
        'const classes = ["card", open && "card-open"].filter(Boolean).join(" ");\n' +
          "<p className={classes}>x</p>",
      ),
    ).toEqual(["card", "card-open"]);
  });

  // A binding that refers to itself is walked once rather than forever.
  it("follows a self-referring binding without looping", () => {
    expect(names('const a = [a, "row"];\n<p className={a}>x</p>')).toEqual([
      "row",
    ]);
  });
});

describe("what a stylesheet declares", () => {
  it("reads the selector and not the rule body", () => {
    const declared = declaredIn(
      `.icon { background: url("./icons/arrow.svg"); }
       .real { color: red; }`,
    );

    expect([...declared].sort()).toEqual(["icon", "real"]);
  });

  it("reads a selector nested inside a rule or a breakpoint", () => {
    const declared = declaredIn(
      `@media (min-width: 40rem) { .wide { display: grid; } }
       .card { &.is-open { color: red; } }`,
    );

    expect([...declared].sort()).toEqual(["card", "is-open", "wide"]);
  });
});

describe("what a browser journey can be shown to walk", () => {
  const walks = (code: string) =>
    selectorClassesIn(parseSource("probe.spec.ts", code));

  it("reads a selector by the call that takes it", () => {
    expect(walks('page.locator("button.auto-row")')).toEqual(["auto-row"]);
    expect(walks('present(page, ".record-tabs")')).toEqual(["record-tabs"]);
  });

  it("reads a selector by its shape, wherever it is handed", () => {
    expect(walks('const sel = ".co-tabs .recordtabs-tab";')).toEqual([
      "co-tabs",
      "recordtabs-tab",
    ]);
  });

  it("reads a template's whole tokens and not its interpolation", () => {
    // biome-ignore lint/suspicious/noTemplateCurlyInString: the placeholder is the subject: this string is the source text the reader parses
    expect(walks('page.locator(`.auto-row[data-id="${id}"]`)')).toEqual([
      "auto-row",
    ]);
  });

  // An `accept` attribute is a list of file extensions in selector characters.
  it("reads a list of file extensions as what it is", () => {
    expect(
      walks('expect(input).toHaveAttribute("accept", ".md,.srt,.pdf")'),
    ).toEqual([]);
  });

  it("reads nothing out of a property access", () => {
    expect(walks("page.locator")).toEqual([]);
  });
});

describe("the classcoverage rule", () => {
  it("reports a planted orphan and a pinned suite through its own corpus", () => {
    const findings = plantedFindings(rule, [
      {
        path: join(srcDir, "features", "p.css"),
        area: "src",
        text: ".row { color: red; }\n",
      },
      {
        path: join(srcDir, "features", "p.tsx"),
        area: "src",
        text: 'export const F = () => <p className="row row-ghost">x</p>;\n',
      },
      {
        path: join(e2eDir, "p.spec.ts"),
        area: "e2e",
        text: 'page.locator(".row .gone-tab");\n',
      },
    ]);
    expect(findings).toEqual([
      "e2e/p.spec.ts walks .gone-tab, which no stylesheet declares; give the element a data-testid and walk that",
      "src/features/p.tsx:1 .row-ghost is matched by no selector in any stylesheet",
    ]);
  });
});
