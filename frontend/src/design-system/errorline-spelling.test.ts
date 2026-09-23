// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  extensionFrontendFiles,
  extensionLayers,
  filesMatching,
  filesUnder,
  parseSource,
  sourceFileAt,
} from "../../scripts/lib/source-tree";
import { rulesIn } from "../testing/css";

// A failure said on one line under a control has one spelling, and it is
// `ErrorLine`. A copy drifts on its own — one keeps the ink and loses the
// announcement, the next the other way round — until a refusal reads like a
// caption. Six arms, one per shape of the copy:
//
//   inline  — a JSX `style` object whose `color` is the literal
//             `var(--dangerText)`.
//   class   — a `className` on any element whose literal text carries the
//             `t-danger` or `form-error` token. `field-error` is Field's own
//             slot and is not this line.
//   sheet   — a `className` carrying a screen class whose every rule sets
//             only the danger ink and margins: `.x-error` is `t-danger`
//             under another name. The list is derived from the screen and
//             extension sheets, because a named list is a second copy of them
//             that misses the next one a screen writes. A design-system sheet
//             owns the ink: its tone classes are a primitive's closed set.
//   message — an intrinsic element whose only child is one
//             `{problemMessageOf(…)}`: a failure said outside ErrorLine. Only
//             an alert owns its cause: a line inside an element, intrinsic or
//             component, carrying a literal `role="alert"` is read out with
//             it. A status or a polite region inserted with its text is often
//             never announced, so a cause under one is still a line. A
//             computed role (a danger `Callout`) does not skip, so a cause
//             inside one takes a waiver.
//   alert   — an intrinsic element carrying `role="alert"` around a LINE:
//             text and expressions only, no element child. A composite — a
//             heading, a glyph, a verb inside the region — is another thing.
//   name    — any identifier `RefusalLine` outside an import.
//
// The message and alert arms read a line by what it SAYS and how it announces;
// the sheet arm reads a class whose whole rule is the ink. A figure or a glyph
// drawn in the ink is not a line, and carries a reasoned `ds:ignore`.
//
// Every arm is held at zero, in every file, wherever the spelling reappears.
//
// The waiver is `ds:ignore <reason>` in a comment on the finding's line or the
// line above it; a marker with no reason does not waive, and is a finding.
// The corpus is derived from the tree and fails closed when small, and every
// arm carries a planted case, because a reader that sees nothing reads green.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const repoRoot = join(frontendRoot, "..");
const sourceRoot = join(frontendRoot, "src");
const extensionsRoot = join(repoRoot, "extensions");

/** The one module allowed to wear the danger ink as a message line. */
const homeModule = "frontend/src/design-system/errorline.tsx";

const advice = "use ErrorLine from design-system/errorline.tsx";

/**
 * The design-system modules that own an alerting line of their own, each with
 * why. Screens take a reasoned `ds:ignore` instead.
 */
const alertOwners = new Map([
  [
    "frontend/src/design-system/atoms.tsx",
    "Field's `field-error` slot, wired to its control's aria-describedby",
  ],
]);

function fromRepo(path: string): string {
  return relative(repoRoot, path).replaceAll("\\", "/");
}

/** Every shipped TSX module, core and extension tier alike, but the home. */
function modules(): string[] {
  return filesUnder(sourceRoot)
    .concat(extensionFrontendFiles(extensionsRoot))
    .map(fromRepo)
    .filter(
      (where) =>
        where.endsWith(".tsx") &&
        !/\.(test|spec|stories)\.tsx$/.test(where) &&
        where !== homeModule,
    );
}

type Arm = "inline" | "class" | "sheet" | "message" | "alert" | "name";

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

function classTokens(attribute: ts.JsxAttribute): string[] {
  return attribute.initializer
    ? fragmentsOf(attribute.initializer).flatMap((fragment) =>
        fragment.split(/\s+/),
      )
    : [];
}

function tagOf(attribute: ts.JsxAttribute): string {
  return attribute.parent.parent.tagName.getText();
}

function dangerClassOn(attribute: ts.JsxAttribute): string | undefined {
  const token = classTokens(attribute).find((name) => dangerClass.test(name));
  return token ? `<${tagOf(attribute)}> carries the ${token} class` : undefined;
}

function sheetClassOn(
  attribute: ts.JsxAttribute,
  sheetClasses: ReadonlySet<string>,
): string | undefined {
  const token = classTokens(attribute).find((name) => sheetClasses.has(name));
  return token
    ? `<${tagOf(attribute)}> carries .${token}, a rule that only sets the danger ink`
    : undefined;
}

const dangerInk = /^color\s*:\s*var\(--dangerText\b[^)]*\)\s*(!important)?$/;

const margin = /^margin(-[a-z]+)*\s*:/;

type Sheet = { where: string; text: string };

const designSystemSheet = /^frontend\/src\/design-system\//;

/**
 * The classes a sheet spells as the danger line: every rule whose selector is
 * that class alone sets the danger ink and margins, and nothing else. One rule
 * that also draws a border or a layout makes the class something else.
 */
function dangerOnlyClasses(sheets: readonly Sheet[]): Set<string> {
  const inked = new Set<string>();
  const other = new Set<string>();
  const spelledBy = sheets.filter(
    ({ where }) => !designSystemSheet.test(where),
  );
  for (const rule of spelledBy.flatMap(({ text }) => rulesIn(text))) {
    if (rule.parents.length > 0) {
      continue;
    }
    const declarations = rule.body
      .split(";")
      .map((declaration) => declaration.trim())
      .filter((declaration) => declaration.length > 0);
    for (const selector of rule.selector.split(",")) {
      const alone = /^\.([\w-]+)$/.exec(selector.trim());
      if (!alone || dangerClass.test(alone[1]) || alone[1] === "field-error") {
        continue;
      }
      // A margin-only rule, a breakpoint's spacing say, leaves the class as it was.
      if (declarations.every((declaration) => margin.test(declaration))) {
        continue;
      }
      const onlyInk = declarations.every(
        (declaration) =>
          dangerInk.test(declaration) || margin.test(declaration),
      );
      (onlyInk ? inked : other).add(alone[1]);
    }
  }
  return new Set([...inked].filter((name) => !other.has(name)));
}

type Opening = ts.JsxOpeningElement | ts.JsxSelfClosingElement;

/** A DOM element rather than a component: a lower-case bare tag name. */
function isIntrinsic(element: Opening): boolean {
  return (
    ts.isIdentifier(element.tagName) && /^[a-z]/.test(element.tagName.text)
  );
}

function carriesAlert(element: Opening): boolean {
  return element.attributes.properties.some((property) => {
    if (!ts.isJsxAttribute(property) || property.name.getText() !== "role") {
      return false;
    }
    const value = property.initializer;
    const literal =
      value && ts.isJsxExpression(value) ? value.expression : value;
    return (
      literal !== undefined &&
      ts.isStringLiteralLike(literal) &&
      literal.text === "alert"
    );
  });
}

/** Whether a JSX element above this one is an alert, which owns its cause. */
function insideAlert(element: ts.JsxElement): boolean {
  for (let at = element.parent; !ts.isSourceFile(at); at = at.parent) {
    if (ts.isJsxElement(at) && carriesAlert(at.openingElement)) {
      return true;
    }
  }
  return false;
}

/** A line: something to read, and no element inside it. */
function isLine(element: ts.JsxElement): boolean {
  const children = ownChildren(element);
  return (
    children.length > 0 &&
    !children.some(
      (child) =>
        ts.isJsxElement(child) ||
        ts.isJsxSelfClosingElement(child) ||
        ts.isJsxFragment(child),
    )
  );
}

function callsProblemMessage(node: ts.Node): boolean {
  if (!ts.isCallExpression(node)) {
    return false;
  }
  const callee = node.expression;
  const name = ts.isPropertyAccessExpression(callee)
    ? callee.name.text
    : callee.getText();
  return name === "problemMessageOf";
}

/** Children that carry something: whitespace between tags is not a child. */
function ownChildren(element: ts.JsxElement): ts.JsxChild[] {
  return element.children.filter(
    (child) => !ts.isJsxText(child) || child.text.trim().length > 0,
  );
}

/** Whether a problemMessageOf call appears anywhere under a node. */
function containsProblemMessage(node: ts.Node): boolean {
  return (
    callsProblemMessage(node) ||
    ts.forEachChild(node, containsProblemMessage) === true
  );
}

/**
 * The names a module gives a problem message: a variable initialised from a
 * call, or a prop handed one. A message usually reaches its element that way,
 * one hop from the call, and a reader of the call alone would miss it.
 */
function messageNames(source: ts.SourceFile): Set<string> {
  const names = new Set<string>();
  const visit = (node: ts.Node): void => {
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.initializer &&
      containsProblemMessage(node.initializer)
    ) {
      names.add(node.name.text);
    }
    if (
      ts.isJsxAttribute(node) &&
      node.initializer &&
      containsProblemMessage(node.initializer)
    ) {
      names.add(node.name.getText());
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return names;
}

function saysOnlyAProblem(element: ts.JsxElement, names: Set<string>): boolean {
  const children = ownChildren(element);
  if (children.length !== 1 || !ts.isJsxExpression(children[0])) {
    return false;
  }
  const said = children[0].expression;
  return (
    said !== undefined &&
    (callsProblemMessage(said) ||
      (ts.isIdentifier(said) && names.has(said.text)))
  );
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
function findingsIn(
  source: ts.SourceFile,
  where: string,
  sheetClasses: ReadonlySet<string> = new Set(),
): (Finding & { waiver: Waiver })[] {
  const lines = source.text.split("\n");
  const out: (Finding & { waiver: Waiver })[] = [];
  const names = messageNames(source);
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
      const sheeted = name === "className" && sheetClassOn(node, sheetClasses);
      if (sheeted) {
        push("sheet", node, sheeted);
      }
    }
    if (
      ts.isJsxElement(node) &&
      isIntrinsic(node.openingElement) &&
      saysOnlyAProblem(node, names) &&
      !insideAlert(node)
    ) {
      push(
        "message",
        node,
        `<${node.openingElement.tagName.getText()}> says a failure message outside ErrorLine`,
      );
    }
    if (
      ts.isJsxElement(node) &&
      isIntrinsic(node.openingElement) &&
      carriesAlert(node.openingElement) &&
      isLine(node) &&
      !alertOwners.has(where)
    ) {
      push(
        "alert",
        node,
        `<${node.openingElement.tagName.getText()}> hand-rolls an alerting line`,
      );
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

type Located = Finding & { where: string; waiver: Waiver };

function shown(finding: Located): string {
  return `${finding.where}:${finding.line}  ${finding.says} — ${advice}`;
}

/** Every stylesheet, core and extension tier alike. */
function sheets(): Sheet[] {
  return filesMatching(sourceRoot, /\.css$/)
    .concat(
      extensionLayers(extensionsRoot).flatMap((layer) =>
        filesMatching(layer, /\.css$/),
      ),
    )
    .map((path) => ({
      where: fromRepo(path),
      text: readFileSync(path, "utf8"),
    }));
}

function spellings() {
  const sheetClasses = dangerOnlyClasses(sheets());
  const all: Located[] = modules().flatMap((where) =>
    findingsIn(sourceFileAt(join(repoRoot, where)), where, sheetClasses).map(
      (finding) => ({
        where,
        ...finding,
      }),
    ),
  );
  const open = all.filter((finding) => finding.waiver !== "reasoned");
  const of = (arm: Arm) =>
    open.filter((finding) => finding.arm === arm).map(shown);
  return {
    inline: of("inline"),
    class: of("class"),
    sheet: of("sheet"),
    name: of("name"),
    message: of("message"),
    alert: of("alert"),
    bare: all.filter((finding) => finding.waiver === "bare").map(shown),
  };
}

describe("a failure line under a control has one spelling", () => {
  const found = spellings();

  it("reads a corpus and an owner that are not empty", () => {
    expect(modules().length).toBeGreaterThan(400);
    expect(sheets().length).toBeGreaterThan(100);
    const home = findingsIn(
      sourceFileAt(join(repoRoot, homeModule)),
      homeModule,
    );
    expect(
      home.filter((finding) => finding.arm === "class").length,
      "errorline.tsx must wear t-danger, or the class arm reads nothing",
    ).toBeGreaterThan(0);
    for (const owner of alertOwners.keys()) {
      const text = readFileSync(join(repoRoot, owner), "utf8");
      expect(text, `${owner} owns no role="alert"; drop it`).toContain(
        'role="alert"',
      );
    }
  });

  it("finds no inline danger colour", () => {
    expect(
      found.inline,
      "each of these paints a message in the danger ink by hand\n",
    ).toEqual([]);
  });

  it("finds no t-danger or form-error class on an element", () => {
    expect(
      found.class,
      "each of these spells the failure line with a class of its own\n",
    ).toEqual([]);
  });

  it("finds no screen class that is only the danger ink", () => {
    expect(
      found.sheet,
      "each of these wears a class whose whole rule is the danger ink\n",
    ).toEqual([]);
  });

  it("finds no problem message said alone outside ErrorLine", () => {
    expect(
      found.message,
      "each of these says a failure outside ErrorLine\n",
    ).toEqual([]);
  });

  it("finds no hand-rolled alerting line", () => {
    expect(
      found.alert,
      "each of these announces a line by hand; it is ErrorLine (standing when it is not news)\n",
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
    const read = (
      text: string,
      where = "frontend/src/screens/x.tsx",
      sheet = "",
    ) =>
      findingsIn(
        parseSource(where, text),
        where,
        dangerOnlyClasses([
          { where: "frontend/src/screens/x.css", text: sheet },
        ]),
      ).map(({ arm, says, waiver }) => `${arm} ${waiver}: ${says}`);

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

    it("reads the danger classes on any element, however assembled", () => {
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

    it("reads the danger class on a small, a cell and a component", () => {
      expect(read('<small className="t-danger">x</small>')).toEqual([
        "class none: <small> carries the t-danger class",
      ]);
      expect(read('<td className="t-num t-danger">x</td>')).toEqual([
        "class none: <td> carries the t-danger class",
      ]);
      expect(read('<Text className="t-danger">x</Text>')).toEqual([
        "class none: <Text> carries the t-danger class",
      ]);
    });

    it("reads a class a sheet spells as the danger ink, and not a box", () => {
      const line = '<p className="t-sub x-error">x</p>';
      expect(
        read(
          line,
          undefined,
          ".x-error { color: var(--dangerText); margin-top: 4px; }",
        ),
      ).toEqual([
        "sheet none: <p> carries .x-error, a rule that only sets the danger ink",
      ]);
      expect(
        read(
          line,
          undefined,
          ".x-error { color: var(--dangerText); border: 1px solid; }",
        ),
      ).toEqual([]);
      expect(
        read(
          line,
          undefined,
          ".x-error { color: var(--dangerText); }\n.x-error { display: flex; }",
        ),
      ).toEqual([]);
      const derives = [
        ".x-error { color: var(--dangerText) !important; }",
        ".x-error { color : var(--dangerText, #b00); }",
        ".x-error { color: var(--dangerText); }\n@media (max-width: 720px) { .x-error { margin-top: 0; } }",
      ];
      for (const sheet of derives) {
        expect(read(line, undefined, sheet), sheet).toHaveLength(1);
      }
      expect(
        read(line, undefined, ".row > .x-error { color: var(--dangerText); }"),
      ).toEqual([]);
      expect(
        dangerOnlyClasses([
          {
            where: "frontend/src/screens/x.css",
            text: ".t-danger { color: var(--dangerText); }\n.x-gap { margin: 0; }",
          },
          {
            where: "frontend/src/design-system/x.css",
            text: ".x-danger { color: var(--dangerText); }",
          },
        ]),
      ).toEqual(new Set());
    });

    it("does not read Field's slot or a longer class", () => {
      expect(read('<p className="field-error">x</p>')).toEqual([]);
      expect(read('<p className="t-danger-strong">x</p>')).toEqual([]);
    });

    it("reads a problem message said alone in an element", () => {
      expect(read("<p>{problemMessageOf(save.error, t)}</p>")).toEqual([
        "message none: <p> says a failure message outside ErrorLine",
      ]);
      expect(
        read(
          '<span className="x">\n  {common.problemMessageOf(e, t)}\n</span>',
        ),
      ).toEqual([
        "message none: <span> says a failure message outside ErrorLine",
      ]);
    });

    it("reads a problem message one hop away, through a variable or a prop", () => {
      expect(
        read(
          "const message = save.isError ? problemMessageOf(save.error, t) : null;\n" +
            "const a = <p>{message}</p>;\n" +
            "const b = <Row previewError={problemMessageOf(e, t)} />;\n" +
            'const c = <p className="row-error">{previewError}</p>;',
        ),
      ).toEqual([
        "message none: <p> says a failure message outside ErrorLine",
        "message none: <p> says a failure message outside ErrorLine",
      ]);
      expect(read('const label = t("x");\n<p>{label}</p>')).toEqual([]);
    });

    it("does not read a problem message among other children, or inside ErrorLine", () => {
      expect(read('<p>{t("x.lead")} {problemMessageOf(e, t)}</p>')).toEqual([]);
      expect(
        read("<p>{problemMessageOf(e, t)}<Button>Retry</Button></p>"),
      ).toEqual([]);
      expect(read("<ErrorLine>{problemMessageOf(e, t)}</ErrorLine>")).toEqual(
        [],
      );
    });

    it("does not read a cause under an alert, and reads it under anything else", () => {
      const cause = "<p>{problemMessageOf(e, t)}</p>";
      const said = "message none: <p> says a failure message outside ErrorLine";
      expect(read(`<div role="alert"><h3>Failed</h3>${cause}</div>`)).toEqual(
        [],
      );
      expect(read(`<Card role="status">${cause}</Card>`)).toEqual([said]);
      expect(
        read(`<div aria-live="polite"><AssistantBubble />${cause}</div>`),
      ).toEqual([said]);
      expect(read(`<div><h3>Failed</h3>${cause}</div>`)).toEqual([said]);
    });

    it("reads an alerting line, however the role and the words are written", () => {
      expect(read('<p role="alert">{t("x")}</p>')).toEqual([
        "alert none: <p> hand-rolls an alerting line",
      ]);
      expect(read('<span role={"alert"}>{msg}</span>')).toEqual([
        "alert none: <span> hand-rolls an alerting line",
      ]);
      // biome-ignore lint/suspicious/noTemplateCurlyInString: a fixture of source code
      expect(read('<div role="alert">Failed: {`${n} rows`}</div>')).toEqual([
        "alert none: <div> hand-rolls an alerting line",
      ]);
    });

    it("does not read a composite alert, another role, or a component", () => {
      expect(
        read('<div role="alert"><strong>Sync stopped.</strong> text</div>'),
      ).toEqual([]);
      expect(read('<div role="alert">\n  <Glyph />\n  {msg}\n</div>')).toEqual(
        [],
      );
      expect(read('<p role="status">x</p>')).toEqual([]);
      expect(read('<Callout role="alert">x</Callout>')).toEqual([]);
    });

    it("lets a design-system owner keep its alert, and only an owner", () => {
      const slot =
        '<p className="field-error" id={errorId} role="alert">{error}</p>';
      expect(read(slot, "frontend/src/design-system/atoms.tsx")).toEqual([]);
      expect(read(slot)).toEqual([
        "alert none: <p> hand-rolls an alerting line",
      ]);
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
    it("reads an announced problem message on both arms", () => {
      expect(read('<p role="alert">{problemMessageOf(e, t)}</p>')).toEqual([
        "message none: <p> says a failure message outside ErrorLine",
        "alert none: <p> hand-rolls an alerting line",
      ]);
    });
  });
});
