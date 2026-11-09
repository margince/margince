// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Native-control gate: no product surface renders a browser-drawn dropdown.
//
// The browser paints a `<select>` itself. Its closed face takes our tokens.
// Its option list does not and cannot: no engine we ship to styles `option`.
// On a screen built from src/design-system/ it reads as a hole in the
// product, and review misses it because the closed control looks right. Use
// `Select` from src/design-system/select.tsx, a button plus a listbox.
//
// There is no exemption. select.tsx names `<select>` only in comments, and a
// parser does not read comments.
//
// All three elements count. `<option>` is what the dropdown is built from,
// and a half-finished move to `Select` leaves it behind.
//
// A native date or time field is the same hole: the browser draws it in its
// own language, not the reader's. Use `DateInput`, `TimeInput` and
// `MonthInput`.
//
// ## Why a parser
//
// Strings, comments, template literals and URLs are properties of the
// language, and TypeScript's parser already knows them. The rule is one
// sentence: no intrinsic `select`, `option` or `optgroup` element.
//
// ## What the parser does not decide
//
// Markup built in a template literal ships too, and the syntax tree cannot
// tell whether a string is rendered. So string content is scanned as text.
// A doc line in a string is then reported; move it to a comment.

import { join, relative } from "node:path";
import ts from "typescript";
import {
  deriveDateHosts,
  literalsOf,
  namesHost,
  publishedFrom,
  renderedName,
  typeInObject,
  typeValues,
} from "../../scripts/lib/input-type";
import {
  extensionPackageLayers,
  filesUnder,
  parseSource,
  scriptKindFor,
} from "../../scripts/lib/source-tree";
import { type CensusRule, extensionsDir, repoRoot, srcDir } from "../tree";

// The elements a native dropdown is made of.
export const nativeControls = new Set(["select", "option", "optgroup"]);

// The pre-filter's names come from the set above, not a second list. A second
// list would miss an element added to the set. A file that spells only the new
// name would be skipped before it is parsed.
//
// The backslash arm is written by hand. It belongs to escapes, not to any
// element name. `'<sel\u0065ct>'` has no `select` in its raw text but cooks to
// one. Every escape starts with a backslash, so a file with neither a name nor
// a backslash cannot spell one.
//
// A name counts only where no word character follows it, in a JSX tag, a
// literal equal to the name, and the string scan. So `options` and `selected`
// are not hits.
export const prefilter = new RegExp(
  `(?:${[...nativeControls].join("|")})(?![A-Za-z0-9_])|\\\\`,
);

// The tags whose `type` reaches a native input as written, derived from the
// design system and every module the extension surface publishes from.
const designSystemDir = join(srcDir, "design-system");
// InlineTextControl sends a date to DateInput, so no date reaches its TextInput.
export const notHosts = new Set(["InlineTextControl"]);
// Each spreads FieldControl or SettingControlProps: an id and aria wiring, no type.
export const clearedSpreads = ["NameDialog", "NumberSetting", "ResolveSheet"];
export const dateHosts = deriveDateHosts(
  [
    ...filesUnder(designSystemDir).filter(
      (file) => !/\.(test|stories)\.tsx?$/.test(file),
    ),
    ...publishedFrom(join(srcDir, "surface", "design-system.ts")),
  ],
  notHosts,
);
// The input types the browser draws in its own language.
const clockTypes = ["date", "time", "datetime-local", "month", "week"];
const clockTypeOf = (values: string[]) =>
  values
    .map((value) => value.toLowerCase())
    .find((value) => clockTypes.includes(value));
const clockAlternation = clockTypes.join("|");
const elementFactories = new Set(["createElement", "jsx", "jsxs", "jsxDEV"]);

// Sound for the same reason as `prefilter`. A hit spells `type`, and its value
// is a literal equal to a clock type, between quotes or backticks.
// Only the string scan reads one unquoted, after `=` and before a non-word
// character. Any other spelling goes through a backslash escape.
const dateValueStart = new RegExp(
  `["'\`](?:${clockAlternation})["'\`]|=\\s*(?:${clockAlternation})(?![\\w-])`,
  "i",
);
export const worthParsing = (text: string) =>
  prefilter.test(text) || (text.includes("type") && dateValueStart.test(text));

// Each native element as a string scan reads it, built once rather than per
// string node.
const tagInString = [...nativeControls].map(
  (tag) => [tag, new RegExp(`<${tag}([^a-zA-Z0-9._:-]|$)`, "g")] as const,
);

// `<input … type="date">`, or any other clock type, built in a string.
const dateMarkup = new RegExp(
  `<input(?![a-zA-Z0-9._:-])[^>]*\\stype\\s*=\\s*(?:"(${clockAlternation})"|'(${clockAlternation})'|(${clockAlternation})(?![\\w-]))`,
  "gi",
);

// Tests and stories are scanned too. A test that drives a native control tests
// the wrong control, and a story catalogues what we ship. The planted
// probes live in census/rules/native-controls.test.ts, outside the tree.

// findNativeControls returns one report line per native dropdown element in the
// source, with the author's own line number. native-controls.test.ts calls it
// on planted source, because a detector that stopped detecting passes a clean
// tree too.
export function findNativeControls(path: string, text: string): string[] {
  // A file that cannot spell any of the names is not parsed: the parse is the
  // census's cost. The filter is sound. A name needs no angle bracket, since
  // `\u003cselect>` has no `<` in its raw text. A backslash alone admits a file,
  // since the string scan reads cooked text. A character reaches cooked text raw
  // or through an escape, and every escape starts with a backslash.
  if (!worthParsing(text)) return [];
  const source = parseSource(path, text);
  const found: string[] = [];
  // Anything not already TSX is read a second time as TSX, and the findings
  // unioned. A file whose JSX the first parse cannot see yields no nodes, which
  // is a silent pass: a `.js` file or a `.ts` file that holds JSX.
  //
  // This is safe: a `.ts` generic read as TSX yields a type name such as `T` as
  // its tag, never `select`, `option` or `optgroup`.
  if (scriptKindFor(path) !== ts.ScriptKind.TSX) {
    for (const hit of findNativeControls(`${path}x`, text)) {
      if (!found.includes(hit)) found.push(hit);
    }
  }
  const at = (node: ts.Node) =>
    source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1;

  // The boundary is any character that cannot continue an element name: anything
  // outside letters, digits, `_`, `-`, `.` and `:`. `[^a-zA-Z0-9]` would read
  // `<select-menu>`, a custom element, as a native control. An allow-list of
  // `\s`, `/` and `>` would miss `/<select[^>]*>/`, where regex syntax follows.
  //
  // An intrinsic element is lowercase. The parser tells it from the component
  // `Select` by node kind, and `<selected>` is a different tag name.
  const intrinsic = (name: ts.JsxTagNameExpression): string | null =>
    ts.isIdentifier(name) && nativeControls.has(name.text) ? name.text : null;

  const visit = (node: ts.Node): void => {
    // Opening and self-closing only. A closing element always has an opening one
    // in parsed JSX, so checking it would report `<option>x</option>` twice.
    if (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) {
      const tag = intrinsic(node.tagName);
      if (tag) found.push(`${at(node)}: <${tag}>`);
      const clock = clockTypeOf(typeValues(node.attributes));
      if (clock && namesHost(dateHosts, node.tagName)) {
        found.push(
          `${at(node)}: <${renderedName(node.tagName)} type="${clock}">`,
        );
      }
    }
    // The same elements built without JSX: `createElement("input", props)`.
    if (
      ts.isCallExpression(node) &&
      elementFactories.has(renderedName(node.expression) ?? "")
    ) {
      const [tag, props] = node.arguments;
      const names = tag ? literalsOf(tag) : [];
      for (const name of names.filter((name) => nativeControls.has(name))) {
        found.push(`${at(node)}: <${name}>`);
      }
      const host = names.includes("input")
        ? "input"
        : tag && namesHost(dateHosts, tag)
          ? renderedName(tag)
          : undefined;
      const clock = props ? clockTypeOf(typeInObject(props)) : undefined;
      if (host && clock) {
        found.push(`${at(node)}: <${host} type="${clock}">`);
      }
    }
    // A string's content is scanned, for the reason in the header. A regex literal
    // is neither a string node nor JSX, and `/<select[^>]*>/` is markup all the
    // same.
    if (
      ts.isStringLiteral(node) ||
      ts.isNoSubstitutionTemplateLiteral(node) ||
      ts.isTemplateHead(node) ||
      ts.isTemplateMiddle(node) ||
      ts.isTemplateTail(node) ||
      ts.isRegularExpressionLiteral(node)
    ) {
      // Detection reads the cooked text and position reads the raw. Cooked, because
      // `\u003cselect>` renders a native dropdown and its raw text has no `<`. Raw,
      // because node.text carries no file offsets.
      //
      // The line is the node's line plus the newlines in the cooked text before the
      // match, counted only when the raw text spans lines. A one-line string with a
      // `\n` escape has cooked newlines the file does not have.
      const cooked = node.text;
      // Both scans below open on a `<`, so a string without one holds no hit.
      // Most strings are copy or contract text, and skip the line arithmetic.
      if (cooked.includes("<")) {
        const spansLines = /\n/.test(node.getText(source));
        const start = source.getLineAndCharacterOfPosition(
          node.getStart(source),
        ).line;
        const lineAt = (index: number) =>
          start +
          1 +
          (spansLines ? (cooked.slice(0, index).match(/\n/g) ?? []).length : 0);
        for (const [tag, inString] of tagInString) {
          // Every occurrence, not the first: a tag a template renders twice
          // reports both of its lines.
          for (const m of cooked.matchAll(inString)) {
            found.push(`${lineAt(m.index)}: <${tag}> inside a string`);
          }
        }
        for (const m of cooked.matchAll(dateMarkup)) {
          const clock = (m[1] ?? m[2] ?? m[3] ?? "").toLowerCase();
          found.push(
            `${lineAt(m.index)}: <input type="${clock}"> inside a string`,
          );
        }
      }
    }
    ts.forEachChild(node, visit);
  };
  ts.forEachChild(source, visit);
  // Deduped at the return: the two parses of a non-TSX file see the same string
  // node at the same offset, and each would report it.
  return [...new Set(found)];
}

export const rule: CensusRule = {
  name: "native-controls",
  // A declaration file emits no code, so it cannot render a control and is
  // not read. The generated contract types are a tenth of the scan's nodes.
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        (file.area === "src" || file.area === "extensions") &&
        file.source !== undefined &&
        !file.path.endsWith(".d.ts"),
    ),
  floors: (corpus) => {
    // An empty scan means the gate reads the wrong tree. The extension layers add a
    // handful of files, too few for the count floor to notice their loss, so each
    // layer is asked for.
    const missing = extensionPackageLayers(extensionsDir)
      .filter((layer) => !corpus.some((f) => f.path.startsWith(`${layer}/`)))
      .map(
        (layer) =>
          `${relative(repoRoot, layer)} has a frontend layer that the census did not reach`,
      );
    return [
      ...(corpus.length > 100 ? [] : ["fewer than 100 files were read"]),
      ...(corpus.some((f) => f.area === "extensions")
        ? []
        : ["the census covered frontend/src but no extension frontend layer"]),
      ...missing,
    ];
  },
  visit: (file) =>
    findNativeControls(file.path, file.text).map((hit) => `${file.rel}:${hit}`),
  advice:
    "Use Select from src/design-system/select.tsx; see src/design-system/README.md. " +
    "In tests, drive it with pickOption from src/design-system/select-testing.ts. " +
    "A date field is DateInput from src/design-system/dateinput.tsx, which a unit imports from @margince/frontend/design-system. " +
    "A time field is TimeInput (timeinput.tsx) and a month field MonthInput (monthinput.tsx).",
};
