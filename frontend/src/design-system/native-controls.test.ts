// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Native-control gate: no product surface may render a browser-drawn dropdown.
//
// A `<select>` is the one control the platform paints for itself. Its closed
// face takes our tokens; the option list behind it — fill, type, highlight,
// scrollbar, and on a phone a whole system sheet — does not, and cannot:
// `option` is not stylable in any engine we ship to. So on a screen built
// entirely from src/design-system/ a native dropdown reads as a hole in the
// product, and the defect is invisible in review because the closed control
// looks correct. The replacement is `Select` in src/design-system/select.tsx —
// a button plus a portalled listbox.
//
// **There is no exemption.** The shell gate this replaces exempted select.tsx
// by full path, and that exemption covered nothing: select.tsx contains no
// native dropdown element. Its three mentions of `<select>` are all in
// COMMENTS, explaining what it exists to replace — and a parser does not see
// comments. So the exemption existed only because a textual scanner might be
// wrong about them, and it blanketed an entire 752-line file to hedge that.
// An exemption that exists because the scanner is uncertain is an exemption
// that can hide a real defect, and this one is simply gone.
//
// All three elements, not just the one that names the gate: `<option>` is what
// the native dropdown is BUILT from, it is meaningless anywhere else, and it is
// what a half-finished migration leaves behind — a screen still handing option
// children to a control that no longer takes them. Catching only `<select>`
// would call that tree clean.
//
// ## Why this is a parser and not a scanner
//
// This replaces frontend/scripts/check-native-controls.sh, which was thirty
// lines of awk implementing a string-and-comment lexer by hand, under a header
// that had to explain its own residues: that a `//` inside a string is not a
// comment, that a URL carries two of them, that a template literal spans lines
// while a quoted string does not, that an unterminated quote must reset so one
// bad line cannot blank the file.
//
// Every one of those is a property of the LANGUAGE, and TypeScript's own parser
// already knows them. None of them is a property of this rule. The rule is one
// sentence — no intrinsic `select`, `option` or `optgroup` element — and it now
// reads as one.
//
// The tree already does this: conformance.test.ts and if-match-coverage.test.ts
// are source-wide fitness functions over `ts.createSourceFile`, and the shell
// gates were the only source-scanning gates that were not.
//
// ## The one thing the parser does NOT decide
//
// Markup can be built inside a template literal, so a `<select>` in a string is
// a native dropdown that would ship. The AST cannot know whether a string is
// rendered, so string CONTENT is scanned textually — deliberately, and stated
// here rather than left as a side effect of how a scanner happened to work.
//
// The cost is a non-rendered reference in a string — a doc line, an example —
// being reported. That is the direction to be wrong in: a false positive is one
// edit away from silence, and a miss is a browser-drawn dropdown nobody sees
// until it is on a screen. A cross-reference belongs in a COMMENT, which the
// parser drops for free, and that is where select.tsx and its neighbours cite
// the thing they exist to remove.

import { existsSync, readdirSync, readFileSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";

const frontendRoot = resolve(__dirname, "../..");
const srcDir = join(frontendRoot, "src");
const extensionsDir = resolve(frontendRoot, "../extensions");

// The elements a native dropdown is made of.
const nativeControls = new Set(["select", "option", "optgroup"]);

// Tests and stories are scanned too: a test that drives a native control is a
// test of the wrong control, and a story catalogues what we ship. The plain-JS
// extensions cost nothing to scan, and a gate whose coverage depends on which
// extension a file happens to carry is a gate with a way around it.
const scanned = /\.(ts|tsx|js|jsx)$/;

// This file holds the planted probes below, which are deliberate examples of
// the markup. Judging them would report the gate's own evidence as a finding.
// Excluded by NAME rather than by "a test file", because a test that renders a
// native control is a test of the wrong control and is still a finding.
const probeFile = join(srcDir, "design-system", "native-controls.test.ts");

function sourceFilesUnder(dir: string): string[] {
  if (!existsSync(dir)) return [];
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) {
      return entry.name === "node_modules" ? [] : sourceFilesUnder(full);
    }
    return scanned.test(entry.name) ? [full] : [];
  });
}

// A unit's screen is shipped UI in the same bundle, so a gate stopping at
// frontend/src would hold the core to a standard the extension tier escapes.
function extensionFrontendFiles(): string[] {
  if (!existsSync(extensionsDir)) return [];
  return readdirSync(extensionsDir, { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .flatMap((entry) =>
      sourceFilesUnder(join(extensionsDir, entry.name, "frontend")),
    );
}

// scriptKindFor picks the dialect, and `.js` is the one that matters.
//
// A `.js` file holding JSX parsed as ScriptKind.TS yields no JSX nodes at all —
// so `<select />` in one was silently missed, which the shell gate this
// replaces did catch. `.ts` cannot be JSX, because `<T>()` there is a type
// argument rather than an element, and asking for TSX would misparse ordinary
// generics.
function scriptKindFor(path: string): ts.ScriptKind {
  if (path.endsWith(".tsx")) return ts.ScriptKind.TSX;
  if (path.endsWith(".jsx") || path.endsWith(".js")) return ts.ScriptKind.JSX;
  return ts.ScriptKind.TS;
}

// findNativeControls returns one report line per native dropdown element in the
// source, with the author's own line number.
//
// Read directly by the probe suite below. A gate asserting a shape is ABSENT
// passes identically over a clean tree and over a detector that has stopped
// detecting, and the shell gate this replaces had no such test at all — its
// hand-written lexer was verified only by the tree happening to be clean.
function findNativeControls(path: string, text: string): string[] {
  const source = ts.createSourceFile(
    path,
    text,
    ts.ScriptTarget.ES2022,
    true,
    scriptKindFor(path),
  );
  const found: string[] = [];
  const at = (node: ts.Node) =>
    source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1;

  // An intrinsic element is lowercase; the design-system component is `Select`,
  // capitalised, and the parser tells the two apart by kind rather than by a
  // pattern that has to guess. `<selected>` is a different tag name entirely,
  // which a textual match had to exclude by hand.
  const intrinsic = (name: ts.JsxTagNameExpression): string | null =>
    ts.isIdentifier(name) && nativeControls.has(name.text) ? name.text : null;

  const visit = (node: ts.Node): void => {
    // Opening and self-closing only. A CLOSING element always has a matching
    // opening one in parsed JSX, so checking it finds nothing new and reports
    // `<option>x</option>` twice. Verified by mutation: removing the closing
    // check changes no verdict, which is what "dead" means.
    if (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) {
      const tag = intrinsic(node.tagName);
      if (tag) found.push(`${at(node)}: <${tag}>`);
    }
    // A string's CONTENT is scanned, for the reason in the header: markup built
    // in a template literal is markup that ships, and the AST cannot know
    // whether a string is rendered.
    if (
      ts.isStringLiteral(node) ||
      ts.isNoSubstitutionTemplateLiteral(node) ||
      ts.isTemplateHead(node) ||
      ts.isTemplateMiddle(node) ||
      ts.isTemplateTail(node)
    ) {
      // The RAW text, not node.text, so an offset into it is an offset into
      // the FILE: a match inside a multi-line template must report the line it
      // is on, not the line the template opened on.
      const raw = node.getText(source);
      for (const tag of nativeControls) {
        const at2 = raw.search(new RegExp(`<${tag}([^a-zA-Z0-9]|$)`));
        if (at2 >= 0) {
          const line =
            source.getLineAndCharacterOfPosition(node.getStart(source) + at2)
              .line + 1;
          found.push(`${line}: <${tag}> inside a string`);
        }
      }
    }
    ts.forEachChild(node, visit);
  };
  ts.forEachChild(source, visit);
  return found;
}

describe("no product surface renders a browser-drawn dropdown", () => {
  it("finds no native select, option or optgroup outside design-system/select.tsx", () => {
    const files = [
      ...sourceFilesUnder(srcDir),
      ...extensionFrontendFiles(),
    ].filter((f) => f !== probeFile);

    // An empty scan means the gate is pointed at the wrong tree. A census that
    // judged nothing certifies nothing, and this one is fail-closed by design.
    expect(files.length).toBeGreaterThan(100);

    const violations = files.flatMap((file) =>
      findNativeControls(file, readFileSync(file, "utf8")).map(
        (hit) => `${relative(frontendRoot, file)}:${hit}`,
      ),
    );

    expect(
      violations,
      "Use Select from src/design-system/select.tsx; see src/design-system/README.md. " +
        "In tests, drive it with pickOption from src/design-system/select-testing.ts.",
    ).toEqual([]);
  });

  it("holds select.tsx to the same rule as everything else", () => {
    // The file this gate points people AT is scanned like any other, and it
    // passes — which is the evidence that the shell gate's exemption was
    // unnecessary rather than merely unused. Its `<select>` mentions are
    // comments citing what it replaces, and a parser drops those for free.
    const path = join(srcDir, "design-system", "select.tsx");
    expect(existsSync(path)).toBe(true);
    expect(findNativeControls(path, readFileSync(path, "utf8"))).toEqual([]);
  });
});

// The half that makes the census above mean anything. Every case is a shape the
// rule must judge, and the shell gate it replaces had no test at all — so its
// hand-written lexer was verified only by the tree happening to be clean.
describe("the native-control detector sees what it claims to", () => {
  const cases: { name: string; fires: boolean; src: string; file?: string }[] =
    [
      { name: "a native select", fires: true, src: "const a = <select />;" },
      {
        name: "an option",
        fires: true,
        src: "const a = <div><option>x</option></div>;",
      },
      {
        name: "an optgroup",
        fires: true,
        src: "const a = <optgroup label='g' />;",
      },
      {
        name: "attributes wrapped onto the next line",
        fires: true,
        src: "const a = (\n  <select\n    id='x'\n  />\n);",
      },
      // The parser tells an intrinsic element from a component by KIND, so the
      // design-system control is not a hit and needs no naming rule.
      {
        name: "the design-system Select",
        fires: false,
        src: "const a = <Select />;",
      },
      {
        name: "a tag merely starting with the same letters",
        fires: false,
        src: "const a = <selectable />;",
      },
      // Comments are not AST nodes, so a cross-reference costs nothing — which is
      // thirty lines of hand-written comment stripping the shell gate needed.
      {
        name: "a cross-reference in a line comment",
        fires: false,
        src: "// replaced <select> with Select\nconst a = 1;",
      },
      {
        name: "a cross-reference in a block comment",
        fires: false,
        src: "/* was <select /> */\nconst a = 1;",
      },
      {
        name: "a cross-reference in a JSX comment",
        fires: false,
        src: "const a = <div>{/* was <select /> */}</div>;",
      },
      // The residues the shell gate's header had to enumerate, each now a
      // property of the parser rather than of the scanner.
      {
        name: "a URL, whose // is not a comment",
        fires: true,
        src: "const u = 'https://h/a//b';\nconst a = <select />;",
      },
      {
        name: "a multi-line template literal followed by markup",
        fires: true,
        src: "const t = `line one\n// not a comment\n`;\nconst a = <select />;",
      },
      {
        name: "an unterminated quote does not blank the file",
        fires: true,
        src: "const s = 'oops;\nconst a = <select />;",
      },
      // Markup built in a string ships, so it is a hit — deliberately, and this
      // is the one rule the parser does not decide.
      {
        name: "markup built in a template literal",
        fires: true,
        src: "const html = `<select><option>x</option></select>`;",
      },
      {
        name: "prose in a string that merely mentions selection",
        fires: false,
        src: "const s = 'pick a selection';",
      },
      // A .ts file has no JSX, and asking for it would be a parse error rather
      // than a finding.
      // A .js file holding JSX is JSX. Parsed as TS it yields no nodes at all,
      // which is a silent miss rather than a false negative you can see.
      {
        name: "JSX in a plain .js file",
        fires: true,
        src: "export const A = () => <select />;",
        file: "probe.js",
      },
      // A hit inside a multi-line template reports ITS line, not the template's.
      {
        name: "a select on the second line of a template",
        fires: true,
        src: "const html = `<form>\n  <select>\n</form>`;",
      },
      {
        name: "a plain .ts file with no markup",
        fires: false,
        src: "export const a = 1;",
        file: "probe.ts",
      },
    ];

  for (const tc of cases) {
    it(`${tc.fires ? "reports" : "ignores"} ${tc.name}`, () => {
      const hits = findNativeControls(tc.file ?? "probe.tsx", tc.src);
      expect(hits.length > 0, `hits: ${JSON.stringify(hits)}`).toBe(tc.fires);
    });
  }
});
