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
import {
  extensionFrontendFiles,
  filesUnder,
  scriptKindFor,
} from "../../scripts/lib/source-tree";

const frontendRoot = resolve(__dirname, "../..");
const repoRoot = resolve(frontendRoot, "..");
const srcDir = join(frontendRoot, "src");
const extensionsDir = resolve(repoRoot, "extensions");

// A template substitution, built rather than written: a literal `${…}` inside
// one of the probe sources below reads to biome as a template curly somebody
// forgot to make a template, and it is right to say so — these are deliberate,
// and building them says which.
const SUBST = "$" + "{id}";

// The elements a native dropdown is made of.
const nativeControls = new Set(["select", "option", "optgroup"]);

// The pre-filter's alternation, DERIVED from the set above rather than restated
// beside it.
//
// It was restated, and that is the defect this file's own subject describes
// wearing different clothes: the prefilter was a second copy of the element
// list, inside the gate whose job is finding second copies. Both lists were
// individually correct, so nothing failed — until a fourth element was added to
// the set and the gate went silently blind to it, because a file spelling only
// `<datalist>` was skipped before it was ever parsed.
//
// The backslash arm is NOT part of the derivation and stays hand-written: it is
// a property of ESCAPES, not of any element name. `'<sel\u0065ct>'` holds no
// verbatim `select` in its raw text but cooks to one, and every escape begins
// with a backslash — so a file with neither a name nor a backslash cannot spell
// one. Deriving it from the set would be wrong, which is why it is stated once
// with its reason instead.
const prefilter = new RegExp(`${[...nativeControls].join("|")}|\\\\`);

// Tests and stories are scanned too: a test that drives a native control is a
// test of the wrong control, and a story catalogues what we ship. Which files
// those are is ../../scripts/lib/source-tree.ts's answer, shared with the
// extension-import gate — the two walks had already drifted to four extensions
// against eight before it existed.

// This file holds the planted probes below, which are deliberate examples of
// the markup. Judging them would report the gate's own evidence as a finding.
// Excluded by NAME rather than by "a test file", because a test that renders a
// native control is a test of the wrong control and is still a finding.
const probeFile = join(srcDir, "design-system", "native-controls.test.ts");

// findNativeControls returns one report line per native dropdown element in the
// source, with the author's own line number.
//
// Read directly by the probe suite below. A gate asserting a shape is ABSENT
// passes identically over a clean tree and over a detector that has stopped
// detecting, and the shell gate this replaces had no such test at all — its
// hand-written lexer was verified only by the tree happening to be clean.
function findNativeControls(path: string, text: string): string[] {
  // A file that can produce none of the three names is not parsed at all. The
  // census reads ~930 files and reparses every non-TSX one as TSX, which took
  // 21s on a CI runner against 1.3s locally and timed the test out; roughly
  // half the tree is skipped here.
  //
  // The filter has to be SOUND, not merely quick, and two escape routes decide
  // its shape. The names carry no angle bracket, because `\u003cselect>` spells
  // one with no `<` in its raw text. And a lone backslash is enough on its own,
  // because the string scan below reads COOKED text: `'<sel\u0065ct>'` holds no
  // verbatim `select` either, and `'<sel\ect>'` cooks to the same markup. A
  // character reaches cooked text only by appearing raw or by coming from an
  // escape, and every escape begins with a backslash — so a file with neither a
  // name nor a backslash cannot spell one, and skipping it costs no coverage.
  if (!prefilter.test(text)) return [];
  const source = ts.createSourceFile(
    path,
    text,
    ts.ScriptTarget.ES2022,
    true,
    scriptKindFor(path),
  );
  const found: string[] = [];
  // Anything not already TSX is read a SECOND time as TSX, and the findings
  // unioned. This is ONE mechanism rather than a per-extension rule, and it
  // exists because a file whose JSX the primary parse cannot see yields ZERO
  // nodes — a silent pass where the shell gate reported it. That happened
  // twice: a `.js` file (found in review) and a `.ts` file holding JSX (found
  // by the next review). Extension-by-extension was the wrong grain; the right
  // question is whether the parser could see the markup at all.
  //
  // Safe because a `.ts` generic reparsed as TSX yields a tag name that is a
  // TYPE identifier — `T`, `K` — and never `select`, `option` or `optgroup`.
  if (scriptKindFor(path) !== ts.ScriptKind.TSX) {
    for (const hit of findNativeControls(`${path}x`, text)) {
      if (!found.includes(hit)) found.push(hit);
    }
  }
  const at = (node: ts.Node) =>
    source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1;

  // The boundary is ANY character that cannot continue an element name —
  // anything outside letters, digits, `_`, `-`, `.` and `:`. Stated that way round
  // because the alternative was tried twice and was wrong both times:
  // `[^a-zA-Z0-9]` accepted a hyphen, so `<select-menu>`, `<option-item>` and
  // `<optgroup-picker>` read as native controls when a custom element is the
  // opposite of one; and an allow-list of `\s`, `/`, `>` was too tight the
  // other way, dropping `/<select[^>]*>/` where regex syntax follows the name.
  //
  // Naming what CONTINUES an identifier is the grammar. Allow-listing what ends
  // one is a guess about what comes next, and there is always another next.
  //
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
    // A regex literal is neither a string node nor JSX, and the shell gate
    // caught one because it read text. `/<select[^>]*>/` is markup a scanner
    // should see for the same reason a template literal is.
    if (
      ts.isStringLiteral(node) ||
      ts.isNoSubstitutionTemplateLiteral(node) ||
      ts.isTemplateHead(node) ||
      ts.isTemplateMiddle(node) ||
      ts.isTemplateTail(node) ||
      ts.isRegularExpressionLiteral(node)
    ) {
      // Detection reads the COOKED text and position reads the RAW, because
      // neither alone is right.
      //
      // Cooked, because an escape evaluates to markup: `\u003cselect>` renders
      // a native dropdown and the raw text has no `<` in it at all. Raw, because
      // node.text carries no file offsets, so a hit in a multi-line template
      // reported the line the template OPENED on.
      //
      // The line is counted from newlines in the cooked text ahead of the
      // match, added to the node's own line — but only when the raw text
      // actually spans lines. A single-line string holding a `\n` escape has
      // newlines in its cooked form that do not exist in the file, and counting
      // those would report a line the reader cannot find.
      const cooked = node.text;
      const spansLines = /\n/.test(node.getText(source));
      const start = source.getLineAndCharacterOfPosition(
        node.getStart(source),
      ).line;
      for (const tag of nativeControls) {
        // EVERY occurrence, not the first: a template rendering the same tag
        // twice on different lines used to report one line and drop the rest.
        for (const m of cooked.matchAll(
          new RegExp(`<${tag}([^a-zA-Z0-9._:-]|$)`, "g"),
        )) {
          const ahead = spansLines
            ? (cooked.slice(0, m.index).match(/\n/g) ?? []).length
            : 0;
          found.push(`${start + ahead + 1}: <${tag}> inside a string`);
        }
      }
    }
    ts.forEachChild(node, visit);
  };
  ts.forEachChild(source, visit);
  // Deduped at the RETURN, not only where the reparse merges in. The reparse
  // checked itself against `found` and the primary parse then appended
  // unconditionally, so every string hit in a non-TSX file was reported twice —
  // the two parses see the same string node at the same offset.
  return [...new Set(found)];
}

describe("no product surface renders a browser-drawn dropdown", () => {
  // A generous timeout, stated rather than defaulted: this walks and parses the
  // whole tree, which is not a unit test's shape of work. The pre-filter above
  // is what keeps it near a second, and the timeout is the floor under a slower
  // runner — CI took 21s before the filter and timed out at 13s.
  it("finds no native select, option or optgroup outside design-system/select.tsx", {
    timeout: 60_000,
  }, () => {
    const files = [
      ...filesUnder(srcDir),
      ...extensionFrontendFiles(extensionsDir),
    ].filter((f) => f !== probeFile);

    // An empty scan means the gate is pointed at the wrong tree. A census that
    // judged nothing certifies nothing, and this one is fail-closed by design.
    expect(files.length).toBeGreaterThan(100);
    // And the extension half specifically. The count floor cannot notice its
    // loss — the layers contribute a handful of files against a floor of 100 —
    // so removing them from the census was a mutant that SURVIVED. A census
    // whose floor only the large half can satisfy cannot see the small half
    // disappear.
    expect(
      files.some((f) => f.startsWith(`${extensionsDir}/`)),
      "the census covered frontend/src but no extension frontend layer",
    ).toBe(true);

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

  it("walks every extension frontend layer, at any depth", () => {
    // The walk had NO test, and three separate mutants to it survived: dropping
    // `.jsx` from the pattern, removing the extension half of the census, and
    // mispointing extensionsDir. The file-count floor could not notice any of
    // them, because the extension layers contribute a handful of files against
    // a floor of 100 — a census whose floor only the large half can satisfy is
    // a census that cannot see the small half disappear.
    const layers = extensionFrontendFiles(extensionsDir);
    expect(
      layers.length,
      "no extension frontend file was found — the extension half of this gate is dark",
    ).toBeGreaterThan(0);
    // Every unit that HAS a frontend layer contributes to it, so a walk that
    // silently stopped at one of them is a failure rather than a smaller number.
    const unitsWithLayers = readdirSync(extensionsDir, { withFileTypes: true })
      .filter((e) => e.isDirectory())
      .map((e) => join(extensionsDir, e.name, "frontend"))
      .filter((layer) => existsSync(layer));
    for (const layer of unitsWithLayers) {
      expect(
        layers.some((f) => f.startsWith(`${layer}/`)),
        `${relative(repoRoot, layer)} has a frontend layer that the walk did not reach`,
      ).toBe(true);
    }
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
  // `expect` is the report LINE, not merely "something fired". Four separate
  // line-number rules survived mutation because every case asserted only
  // `hits.length > 0` — including the one NAMED "reports ITS line". A case
  // that cannot fail on the thing it is named for is the defect this file
  // exists to stop shipping.
  const cases: {
    name: string;
    fires: boolean;
    src: string;
    file?: string;
    expect?: string[];
  }[] = [
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
      expect: ["2: <select> inside a string"],
    },
    {
      name: "a plain single-quoted string holding markup",
      fires: true,
      src: "const html = '<select>';",
      expect: ["1: <select> inside a string"],
    },
    {
      name: "a template with a substitution",
      fires: true,
      src: "const html = `<select id=" + SUBST + ">`;",
      expect: ["1: <select> inside a string"],
    },
    {
      name: "a substitution's TAIL, on its own line",
      fires: true,
      src: "const html = `" + SUBST + "\n<option>`;",
      expect: ["2: <option> inside a string"],
    },
    // The word boundary in the string scan: prose is not markup.
    {
      name: "a string mentioning selectable",
      fires: false,
      src: "const s = '<selectable/>';",
    },
    // A regex literal is neither a string node nor JSX, and the shell gate
    // saw it because it read text.
    {
      name: "an escape that evaluates to markup",
      fires: true,
      // The raw text has no `<` at all; the cooked text renders a native
      // dropdown. Scanning raw missed it entirely.
      src: "const html = `\\u003cselect>`;",
      expect: ["1: <select> inside a string"],
    },
    {
      name: "an escape that spells the NAME, not just the bracket",
      fires: true,
      // The pre-filter reads raw text and the scan reads cooked, so escaping
      // the name is a way past a filter that only looks for the name. Nothing
      // here is exotic: `\e` cooks to `e` too, which is why the filter treats
      // any backslash as reason enough to parse.
      src: "const html = `<sel\\u0065ct>`;",
      expect: ["1: <select> inside a string"],
    },
    {
      name: "the same tag twice, on different lines of one template",
      fires: true,
      // search() returned only the first, so the second line was dropped
      // silently — a report naming one of two is one a reader trusts to be
      // complete.
      src: "const html = `<select>\n<select>`;",
      expect: ["1: <select> inside a string", "2: <select> inside a string"],
    },
    {
      name: "a newline escape in a single-line string does not move the line",
      fires: true,
      // Cooked newlines that do not exist in the FILE must not be counted,
      // or the report names a line the reader cannot find.
      src: "const a = 1;\nconst html = '\\n<select>';",
      expect: ["2: <select> inside a string"],
    },
    // A hyphen CONTINUES an element name, so these are custom elements —
    // the opposite of native ones. The first boundary spelling accepted a
    // hyphen and reported all three.
    {
      name: "a custom element named select-menu",
      fires: false,
      src: "const a = <select-menu />;",
    },
    {
      name: "a custom element in a string",
      fires: false,
      src: "const h = '<option-item></option-item>';",
    },
    // Two parses see the same string node at the same offset, so a hit in a
    // non-TSX file was reported twice.
    {
      name: "a string hit in a .ts file, reported once",
      fires: true,
      src: "export const h = '<select>';",
      file: "probe.ts",
      expect: ["1: <select> inside a string"],
    },
    {
      name: "markup inside a regex literal",
      fires: true,
      src: "const re = /<select[^>]*>/;",
      expect: ["1: <select> inside a string"],
    },
    // A .ts file cannot legally hold JSX; parsed as TS it yields zero nodes,
    // which is a silent pass rather than a finding.
    {
      name: "JSX in a .ts file, which cannot legally hold it",
      fires: true,
      src: "export const a = <select />;",
      file: "probe.ts",
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
      if (tc.expect) expect(hits).toEqual(tc.expect);
    });
  }

  // The case that is actually owed, and it is not "the prefilter is right".
  //
  // Both lists were individually correct while nothing bound them, so every
  // test anybody would think to write passed. What has to be asserted is the
  // DERIVATION: that a name added to the declared set reaches the prefilter
  // without anybody remembering to edit a regex.
  //
  // Written against the set rather than against a literal, so it cannot itself
  // become a third copy.
  it("derives its pre-filter from the declared set, so a fourth element is not invisible", () => {
    for (const tag of nativeControls) {
      expect(
        prefilter.test(`const a = <${tag} />;`),
        `the pre-filter does not admit a file spelling <${tag}>`,
      ).toBe(true);
    }
    // And a name that is NOT in the set must not be admitted by the name arm —
    // otherwise "derived" would be satisfied by a regex matching everything.
    expect(prefilter.test("const a = <div />;")).toBe(false);
    // The backslash arm is separate and stays: an escape can spell any of the
    // names without their letters appearing raw.
    expect(prefilter.test("const a = '<sel\\u0065ct>';")).toBe(true);
  });
});
