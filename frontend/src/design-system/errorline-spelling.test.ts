// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
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
// `ErrorLine`. A copy drifts on its own — one keeps the ink and loses the
// announcement, the next the other way round — until a refusal reads like a
// caption. Five arms, one per shape of the copy:
//
//   inline  — a JSX `style` object whose `color` is the literal
//             `var(--dangerText)`.
//   class   — a `className` on any element whose literal text carries the
//             `t-danger` or `form-error` token. `field-error` is Field's own
//             slot and is not this line.
//   message — an intrinsic element whose only child is one
//             `{problemMessageOf(…)}`: a failure said outside ErrorLine.
//   alert   — an intrinsic element carrying `role="alert"` around a LINE:
//             text and expressions only, no element child. A composite — a
//             heading, a glyph, a verb inside the region — is another thing.
//   name    — any identifier `RefusalLine` outside an import.
//
// Stylesheets are not read: `color: var(--dangerText)` in a screen sheet also
// marks a figure or a glyph, so the message line is caught by what it SAYS
// (message) and how it announces (alert) rather than by its colour.
//
// `inline`, `class` and `name` are held at zero. `message` and `alert` are a
// CENSUS that only falls: BASELINE pins how many such lines each file carried
// when the gate was armed, a line both arms read counts once, and a new file
// or a higher count fails. A file under its entry fails too, until the entry
// is lowered in the same change. The standing entries are the sweep a
// follow-up issue tracks; this directory takes no entry at all.
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

type Arm = "inline" | "class" | "message" | "alert" | "name";

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

function dangerClassOn(attribute: ts.JsxAttribute): string | undefined {
  const tag = attribute.parent.parent.tagName.getText();
  if (!attribute.initializer) {
    return undefined;
  }
  const token = fragmentsOf(attribute.initializer)
    .flatMap((fragment) => fragment.split(/\s+/))
    .find((name) => dangerClass.test(name));
  return token ? `<${tag}> carries the ${token} class` : undefined;
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
    }
    if (
      ts.isJsxElement(node) &&
      isIntrinsic(node.openingElement) &&
      saysOnlyAProblem(node, names)
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

/**
 * The message and alert lines standing when the gate was armed, per file.
 * EXACT in both directions, the way `inlinelayout.test.ts` holds its count.
 */
const BASELINE = new Map<string, number>([
  ["extensions/openchannel/frontend/endpointcard.tsx", 3],
  ["frontend/src/screens/analytics.tsx", 1],
  ["frontend/src/screens/auth.tsx", 1],
  ["frontend/src/screens/automations.tsx", 3],
  ["frontend/src/screens/backfill.tsx", 1],
  ["frontend/src/screens/book.tsx", 2],
  ["frontend/src/screens/brief.decisions.tsx", 1],
  ["frontend/src/screens/capture-senders.tsx", 1],
  ["frontend/src/screens/common.tsx", 1],
  ["frontend/src/screens/companycontacts/introrequest.tsx", 1],
  ["frontend/src/screens/companydossier.tsx", 1],
  ["frontend/src/screens/companygrowthfit.tsx", 1],
  ["frontend/src/screens/companyvatmark.tsx", 1],
  ["frontend/src/screens/composehead.tsx", 1],
  ["frontend/src/screens/connect-posture.tsx", 1],
  ["frontend/src/screens/contactemploymentrow.tsx", 1],
  ["frontend/src/screens/contactnetwork/edgedetail.tsx", 1],
  ["frontend/src/screens/contactnetwork/index.tsx", 1],
  ["frontend/src/screens/emailaccesseditor.tsx", 1],
  ["frontend/src/screens/employmentimport.tsx", 1],
  ["frontend/src/screens/filterexport.tsx", 1],
  ["frontend/src/screens/imap-connect-form.tsx", 1],
  ["frontend/src/screens/installation-setup.tsx", 1],
  ["frontend/src/screens/leads.tsx", 1],
  ["frontend/src/screens/leadsignals.tsx", 1],
  ["frontend/src/screens/leadvocab.tsx", 1],
  ["frontend/src/screens/listquery.tsx", 1],
  ["frontend/src/screens/noticecases.tsx", 2],
  ["frontend/src/screens/onboarding-backread.tsx", 5],
  ["frontend/src/screens/onboarding-connect-panels.tsx", 1],
  ["frontend/src/screens/onboarding-conversation/basis-act.tsx", 1],
  ["frontend/src/screens/onboarding-conversation/company-act.tsx", 2],
  ["frontend/src/screens/onboarding-conversation/connect-act.tsx", 1],
  ["frontend/src/screens/onboarding-conversation/team-act.tsx", 1],
  ["frontend/src/screens/onboarding-conversation/voice-act.tsx", 1],
  ["frontend/src/screens/onboarding-conversation/voice-scenes.tsx", 1],
  ["frontend/src/screens/onboarding-conversation/way-onward.tsx", 1],
  ["frontend/src/screens/onboarding-gate.tsx", 1],
  ["frontend/src/screens/onboarding-read.tsx", 1],
  ["frontend/src/screens/privacy.corrections.tsx", 2],
  ["frontend/src/screens/privacy.tsx", 2],
  ["frontend/src/screens/rate-refresh.tsx", 1],
  ["frontend/src/screens/restrictedrecords.tsx", 1],
  ["frontend/src/screens/retention.tsx", 3],
  ["frontend/src/screens/retentionpolicyform.tsx", 1],
  ["frontend/src/screens/settings.exitcriteria.tsx", 1],
  ["frontend/src/screens/settings.tsx", 2],
  ["frontend/src/screens/setupclaim.tsx", 1],
  ["frontend/src/screens/taskactions.tsx", 1],
  ["frontend/src/screens/voice-dna.tsx", 4],
  ["frontend/src/screens/voice-versions.tsx", 2],
  ["frontend/src/screens/worklist.plan.tsx", 1],
  ["frontend/src/screens/worklist.tsx", 1],
]);

/** What the census carries across the tree, so a rise is one number. */
const TOTAL = 73;

/** The tier that publishes the alternative takes no entry. */
const HELD_AT_ZERO = /^frontend\/src\/design-system\//;

type Located = Finding & { where: string; waiver: Waiver };

const censusArms: readonly Arm[] = ["message", "alert"];

/** Each census line once, however many of the census arms read it. */
function standingLines(found: readonly Located[]): Located[] {
  const seen = new Map<string, Located>();
  for (const finding of found) {
    const key = `${finding.where}:${finding.line}`;
    if (censusArms.includes(finding.arm) && !seen.has(key)) {
      seen.set(key, finding);
    }
  }
  return [...seen.values()];
}

function countsByFile(lines: readonly Located[]): Map<string, number> {
  const counts = new Map<string, number>();
  for (const line of lines) {
    counts.set(line.where, (counts.get(line.where) ?? 0) + 1);
  }
  return counts;
}

function shown(finding: Located): string {
  return `${finding.where}:${finding.line}  ${finding.says} — ${advice}`;
}

/** Every line in a file over its entry, named; a file with none is over at 0. */
function overBaseline(
  lines: readonly Located[],
  baseline: ReadonlyMap<string, number>,
): string[] {
  return [...countsByFile(lines)]
    .filter(([where, count]) => count > (baseline.get(where) ?? 0))
    .flatMap(([where]) => {
      const entry = baseline.get(where);
      const why =
        entry === undefined ? "not in BASELINE" : `over its entry of ${entry}`;
      return lines
        .filter((line) => line.where === where)
        .map((line) => `${shown(line)} (${why})`);
    });
}

function behindBaseline(
  lines: readonly Located[],
  baseline: ReadonlyMap<string, number>,
): string[] {
  const counts = countsByFile(lines);
  return [...baseline]
    .filter(([where, allowed]) => (counts.get(where) ?? 0) < allowed)
    .map(([where, allowed]) => {
      const count = counts.get(where) ?? 0;
      return count === 0
        ? `${where}: carries none — remove the entry`
        : `${where}: carries ${count}, not ${allowed} — lower the entry to ${count}`;
    });
}

function census() {
  const all: Located[] = modules().flatMap((where) =>
    findingsIn(sourceFileAt(join(repoRoot, where)), where).map((finding) => ({
      where,
      ...finding,
    })),
  );
  const open = all.filter((finding) => finding.waiver !== "reasoned");
  const of = (arm: Arm) =>
    open.filter((finding) => finding.arm === arm).map(shown);
  return {
    inline: of("inline"),
    class: of("class"),
    name: of("name"),
    standing: standingLines(open),
    bare: all.filter((finding) => finding.waiver === "bare").map(shown),
  };
}

describe("a failure line under a control has one spelling", () => {
  const found = census();

  it("reads a corpus and an owner that are not empty", () => {
    expect(modules().length).toBeGreaterThan(400);
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

  it("carries the census total it is pinned at", () => {
    expect(found.standing.length).toBe(TOTAL);
  });

  it("carries no message or alert line over its file's entry", () => {
    const over = overBaseline(found.standing, BASELINE);
    expect(
      over,
      "each of these says a failure outside ErrorLine or announces a line by " +
        "hand; it is ErrorLine (standing when it is not news)\n",
    ).toEqual([]);
  });

  it("keeps no census entry above what the tree carries", () => {
    const behind = behindBaseline(found.standing, BASELINE);
    expect(behind, behind.join("\n")).toEqual([]);
  });

  it("baselines nothing in this directory", () => {
    const refused = [...BASELINE.keys()].filter((where) =>
      HELD_AT_ZERO.test(where),
    );
    expect(refused, "the design system takes no census entry\n").toEqual([]);
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
    const read = (text: string, where = "frontend/src/screens/x.tsx") =>
      findingsIn(parseSource(where, text), where).map(
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
    it("fails a fresh file's one message line as not in BASELINE, and counts a doubly-read line once", () => {
      const where = "frontend/src/screens/fresh.tsx";
      const lines = standingLines(
        findingsIn(
          parseSource(where, '<p role="alert">{problemMessageOf(e, t)}</p>'),
          where,
        ).map((finding) => ({ where, ...finding })),
      );
      expect(lines).toHaveLength(1);
      expect(overBaseline(lines, BASELINE)).toEqual([
        `${where}:1  <p> says a failure message outside ErrorLine — ${advice} (not in BASELINE)`,
      ]);
      expect(overBaseline(lines, new Map([[where, 1]]))).toEqual([]);
      expect(behindBaseline([], new Map([[where, 1]]))).toEqual([
        `${where}: carries none — remove the entry`,
      ]);
    });
  });
});
