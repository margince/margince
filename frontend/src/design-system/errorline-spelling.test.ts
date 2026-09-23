// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  extensionFrontendFiles,
  filesUnder,
  parseSource,
  sourceFileAt,
} from "../../scripts/lib/source-tree";

// A failure said on one line under a control has one spelling, and it is
// `ErrorLine`. The tree grew four: an inline `color: var(--dangerText)`, a
// `t-danger` or `form-error` class on a bare element, and a `RefusalLine` kept
// in a screen module. Each drifted on its own — half lacked the announcement,
// some the ink — so a refusal and a caption read the same. Three arms:
//
//   inline — a JSX `style` object whose `color` is the literal
//            `var(--dangerText)`.
//   class  — a `className` on a `p`, `span` or `div` whose literal text carries
//            the `t-danger` or `form-error` token. `field-error` is Field's own
//            slot and is not this line.
//   name   — any identifier `RefusalLine` outside an import: the retired
//            spelling, declared or rendered.
//
// The waiver is `ds:ignore <reason>` in a comment on the finding's line or the
// line above it; a marker with no reason does not waive, and is a finding.
// The corpus is derived from the tree and fails closed when small, and every
// arm carries a planted case, because a reader that sees nothing reads green.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const sourceRoot = join(frontendRoot, "src");
const extensionsRoot = join(frontendRoot, "..", "extensions");

/** The one module allowed to wear the danger ink as a message line. */
const homeModule = "src/design-system/errorline.tsx";

const advice = "use ErrorLine from design-system/errorline.tsx";

function fromFrontend(path: string): string {
  return relative(frontendRoot, path).replaceAll("\\", "/");
}

/** Every shipped TSX module, core and extension tier alike, but the home. */
function modules(): string[] {
  return filesUnder(sourceRoot)
    .concat(extensionFrontendFiles(extensionsRoot))
    .map(fromFrontend)
    .filter(
      (where) =>
        where.endsWith(".tsx") &&
        !/\.(test|spec|stories)\.tsx$/.test(where) &&
        where !== homeModule,
    );
}

type Arm = "inline" | "class" | "name";

type Finding = { arm: Arm; line: number; says: string };

type Waiver = "none" | "reasoned" | "bare";

/** The marker on a line, and whether it gives a reason. */
function waiverOn(text: string): Waiver {
  const marker = /ds:ignore\b(.*)$/.exec(text);
  if (!marker) {
    return "none";
  }
  const reason = marker[1].replaceAll(/\*\/|\}/g, "").trim();
  return reason.length > 0 ? "reasoned" : "bare";
}

/** The waiver that sits with a finding: its own line first, then the one above. */
function waiverFor(lines: readonly string[], line: number): Waiver {
  const own = waiverOn(lines[line - 1] ?? "");
  return own === "none" ? waiverOn(lines[line - 2] ?? "") : own;
}

/** The literal text a node can evaluate to: strings and template pieces. */
function fragmentsOf(node: ts.Node): string[] {
  const out: string[] = [];
  const collect = (child: ts.Node) => {
    if (ts.isStringLiteral(child) || ts.isTemplateLiteralToken(child)) {
      out.push(child.text);
    }
    ts.forEachChild(child, collect);
  };
  collect(node);
  return out;
}

const dangerClass = /^(t-danger|form-error)$/;

const bareElement = /^(p|span|div)$/;

function inlineDanger(attribute: ts.JsxAttribute): boolean {
  const value = attribute.initializer;
  if (!value || !ts.isJsxExpression(value) || !value.expression) {
    return false;
  }
  const style = value.expression;
  return (
    ts.isObjectLiteralExpression(style) &&
    style.properties.some(
      (member) =>
        ts.isPropertyAssignment(member) &&
        member.name.getText().replaceAll(/["']/g, "") === "color" &&
        ts.isStringLiteralLike(member.initializer) &&
        member.initializer.text.trim() === "var(--dangerText)",
    )
  );
}

function dangerClassOn(attribute: ts.JsxAttribute): string | undefined {
  const tag = attribute.parent.parent.tagName.getText();
  if (!bareElement.test(tag) || !attribute.initializer) {
    return undefined;
  }
  const token = fragmentsOf(attribute.initializer)
    .flatMap((fragment) => fragment.split(/\s+/))
    .find((name) => dangerClass.test(name));
  return token ? `<${tag}> carries the ${token} class` : undefined;
}

function insideImport(node: ts.Node): boolean {
  for (let at: ts.Node | undefined = node; at; at = at.parent) {
    if (ts.isImportDeclaration(at)) {
      return true;
    }
  }
  return false;
}

/** Every finding in one module, waived or not, each with its waiver. */
function findingsIn(source: ts.SourceFile): (Finding & { waiver: Waiver })[] {
  const lines = source.text.split("\n");
  const out: (Finding & { waiver: Waiver })[] = [];
  const push = (arm: Arm, node: ts.Node, says: string) => {
    const line =
      source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1;
    out.push({ arm, line, says, waiver: waiverFor(lines, line) });
  };
  const visit = (node: ts.Node): void => {
    if (ts.isJsxAttribute(node)) {
      const name = node.name.getText();
      if (name === "style" && inlineDanger(node)) {
        push("inline", node, "style colours the text var(--dangerText)");
      }
      const classed = name === "className" && dangerClassOn(node);
      if (classed) {
        push("class", node, classed);
      }
    }
    if (
      ts.isIdentifier(node) &&
      node.text === "RefusalLine" &&
      !insideImport(node)
    ) {
      push("name", node, "RefusalLine is the retired spelling");
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return out;
}

function census() {
  const all = modules().flatMap((where) =>
    findingsIn(sourceFileAt(join(frontendRoot, where))).map((finding) => ({
      where,
      ...finding,
    })),
  );
  const shown = (f: (typeof all)[number]) =>
    `${f.where}:${f.line}  ${f.says} — ${advice}`;
  const open = all.filter((finding) => finding.waiver !== "reasoned");
  const of = (arm: Arm) =>
    open.filter((finding) => finding.arm === arm).map(shown);
  return {
    inline: of("inline"),
    class: of("class"),
    name: of("name"),
    bare: all.filter((finding) => finding.waiver === "bare").map(shown),
  };
}

describe("a failure line under a control has one spelling", () => {
  const found = census();

  it("reads a corpus and an owner that are not empty", () => {
    expect(modules().length).toBeGreaterThan(400);
    const home = findingsIn(sourceFileAt(join(frontendRoot, homeModule)));
    expect(
      home.filter((finding) => finding.arm === "class").length,
      "errorline.tsx no longer wears t-danger, so the class arm reads nothing",
    ).toBeGreaterThan(0);
  });

  it("finds no inline danger colour", () => {
    expect(
      found.inline,
      "each of these paints a message in the danger ink by hand\n",
    ).toEqual([]);
  });

  it("finds no t-danger or form-error class on a bare element", () => {
    expect(
      found.class,
      "each of these spells the failure line with a class of its own\n",
    ).toEqual([]);
  });

  it("finds no RefusalLine", () => {
    expect(found.name, "RefusalLine is retired\n").toEqual([]);
  });

  it("finds no waiver without a reason", () => {
    expect(
      found.bare,
      "ds:ignore says a line was skipped and not why; write the reason after it\n",
    ).toEqual([]);
  });

  describe("the detector", () => {
    const read = (text: string) =>
      findingsIn(parseSource("x.tsx", text)).map(
        ({ arm, says, waiver }) => `${arm} ${waiver}: ${says}`,
      );

    it("reads an inline danger colour, and not another colour", () => {
      expect(
        read('<p style={{ color: "var(--dangerText)", marginTop: 8 }}>x</p>'),
      ).toEqual(["inline none: style colours the text var(--dangerText)"]);
      expect(
        read('<p style={{ color: "var(--textSecondary)" }}>x</p>'),
      ).toEqual([]);
      expect(
        read('<p style={{ background: "var(--dangerText)" }}>x</p>'),
      ).toEqual([]);
    });

    it("reads the danger classes on a bare element, however assembled", () => {
      expect(read('<p className="t-danger">x</p>')).toEqual([
        "class none: <p> carries the t-danger class",
      ]);
      expect(read('<span className="t-caption form-error">x</span>')).toEqual([
        "class none: <span> carries the form-error class",
      ]);
      expect(read('<div className={cx("t-sub", "t-danger")}>x</div>')).toEqual([
        "class none: <div> carries the t-danger class",
      ]);
    });

    it("does not read Field's slot, a longer class, or another element", () => {
      expect(read('<p className="field-error">x</p>')).toEqual([]);
      expect(read('<p className="t-danger-strong">x</p>')).toEqual([]);
      expect(read('<td className="t-num t-danger">x</td>')).toEqual([]);
    });

    it("reads RefusalLine declared and rendered, and not imported", () => {
      expect(
        read(
          'import { RefusalLine } from "./common";\n' +
            "export function RefusalLine() { return null; }\n" +
            "const x = <RefusalLine error={e} />;",
        ),
      ).toEqual([
        "name none: RefusalLine is the retired spelling",
        "name none: RefusalLine is the retired spelling",
      ]);
    });

    it("reads a waiver on the line or above it, and a bare one as bare", () => {
      expect(
        read(
          "<>\n  {/* ds:ignore the host page owns this ink */}\n" +
            '  <p className="t-danger">x</p>\n' +
            '  <p className="t-danger">y</p> {/* ds:ignore */}\n' +
            '  <p style={{ color: "var(--dangerText)" }}>z</p> // ds:ignore embedded widget\n</>',
        ),
      ).toEqual([
        "class reasoned: <p> carries the t-danger class",
        "class bare: <p> carries the t-danger class",
        "inline reasoned: style colours the text var(--dangerText)",
      ]);
    });
  });
});
