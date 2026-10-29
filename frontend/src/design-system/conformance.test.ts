// biome-ignore-all lint/suspicious/noTemplateCurlyInString: these strings are
// FIXTURES of source code, and the ${...} in them is the subject under test —
// a template literal the sweep has to recognize, not one this file meant to
// interpolate.
import { readFileSync } from "node:fs";
import { join, relative } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  files,
  frontendRoot,
  scanBudget,
} from "../../scripts/lib/conformance-corpus";
import {
  parseSource,
  sourceFileAt,
  sourceTextAt,
} from "../../scripts/lib/source-tree";

// The two source-wide design gates from B-EP09.1, derived from the tree so a
// new file is enrolled the moment it exists:
//  - exactly three type families (Outfit / Geist / Geist Mono, §2) — any
//    other font-family fails the build;
//  - every colour reads from a token — literal colours live only in tokens.css.

const allowedFamilies = new Set([
  "Outfit",
  "Geist",
  "Geist Mono",
  // stack fallbacks named in the §2 token definitions
  "system-ui",
  "sans-serif",
  "ui-monospace",
  "monospace",
]);

// The value of a JSX attribute when the source states it as a literal string.
// The role gate below needs the value itself, not the initializer's text: a
// substring test for `status` reads `role={role}` as some other role and
// `role="statusbar"` as `status`, wrong in both directions. Anything computed —
// a variable, a call, a template with a substitution — has no value this file
// can read, and returns undefined.
function literalAttributeValue(initializer: ts.Node): string | undefined {
  if (
    ts.isStringLiteral(initializer) ||
    ts.isNoSubstitutionTemplateLiteral(initializer)
  ) {
    return initializer.text;
  }
  if (ts.isJsxExpression(initializer) && initializer.expression !== undefined) {
    return literalAttributeValue(initializer.expression);
  }
  return undefined;
}

// A colour written out rather than read from a token: any hex form CSS accepts
// (#rgb, #rgba, #rrggbb, #rrggbbaa) or a raw colour function.
const LITERAL_COLOUR = /#[0-9a-fA-F]{3,8}\b|\b(?:rgba?|hsla?|oklch)\(/;

// Blanks every span a pattern matches, keeping the newlines so a finding's
// line number still points at the right line. Used for the languages whose
// comments a regex CAN settle — CSS and HTML have one comment form each and no
// line comment, so there is no `//`-inside-a-URL problem to parse around.
function blankSpans(text: string, comment: RegExp): string {
  return text.replace(comment, (span) => span.replace(/[^\n]/g, " "));
}

// The source the colour gate actually reads: TypeScript with its comments
// blanked out. A comment is prose, and prose carries issue references — `#2463`
// is four hex digits, which is exactly the `#rgba` short form, so a gate that
// reads comments names an innocent line for the right shape in the wrong place.
//
// Which spans are comments comes from the parser rather than from a pattern,
// because a comment opener is not something a pattern can settle: `//` sits
// inside every URL and `/*` inside path strings. Blanked rather than deleted,
// keeping the newlines, because the finding carries a line number.
//
// The walk descends to the TOKENS, and that is what reaches a JSX expression
// comment: `{/* … */}` parses as a JsxExpression with no expression, and its
// text is leading trivia of the closing brace — a walk over declaration
// positions alone never asks that token anything.
//
// A second spelling of format/zone-by-purpose.test.ts's `code()`, deliberately:
// that one lives inside a test file, and importing it here would run that
// file's whole suite as a side effect of the import.
function scannableSource(file: string, text: string): string {
  if (!/\.tsx?$/.test(file)) {
    // CSS and HTML carry prose too, and the same reference trips the same
    // pattern there: `/* see #2463 */` is four hex digits to the colour rule,
    // and a family named inside a comment is a family nobody ships. Neither
    // language has a line comment, so one span each — blanked, not deleted,
    // for the line numbers.
    return blankSpans(
      text,
      file.endsWith(".css") ? /\/\*[\s\S]*?\*\//g : /<!--[\s\S]*?-->/g,
    );
  }
  const parsed = parseSource(file, text);
  const chars = text.split("");
  const blank = ({ pos, end }: ts.CommentRange): void => {
    for (let index = pos; index < end; index++) {
      if (chars[index] !== "\n") {
        chars[index] = " ";
      }
    }
  };
  // Leading AND trailing: the parser calls a comment that shares a line with
  // the code before it trailing, and reports it under that name only.
  const strip = (node: ts.Node): void => {
    ts.getLeadingCommentRanges(text, node.pos)?.forEach(blank);
    // `node.end` is where a trailing comment can begin, which is what the
    // API means by the name. It changes no outcome here — the walk reaches
    // every token, so the same span is also leading trivia of the next one —
    // and it is the spelling that says what it is doing.
    ts.getTrailingCommentRanges(text, node.end)?.forEach(blank);
    for (const child of node.getChildren(parsed)) {
      strip(child);
    }
  };
  strip(parsed);
  return chars.join("");
}

/**
 * Every type family named in one source text, in either spelling.
 *
 * A stylesheet declares `font-family:` and a TSX style prop spells the same
 * thing `fontFamily:`, so the pattern carries two alternatives — and only ONE
 * capture group can fire on a given hit. Reading the first alone made every
 * inline `fontFamily` a match whose families were `undefined` and silently
 * skipped: the gate said PASS about a spelling it had never once looked at.
 *
 * The inline arm takes either quote, because the quote is the formatter's
 * choice and not the author's, and the declared arm runs to the `;`/`}` rather
 * than stopping at a quote — a CSS family whose name needs quoting is exactly
 * the multi-word kind this rule is about, and stopping short made
 * `font-family: "Comic Sans MS"` invisible while `font-family: Outfit, "DM
 * Sans"` was read as naming only Outfit. A gate that only sees what biome happens to
 * emit today stops seeing the day that changes — and it is the same shape of
 * miss as the capture group: under-recognition, reported as PASS.
 */
function familiesIn(text: string): string[] {
  const found: string[] = [];
  for (const [, declared, quoted, templated] of text.matchAll(
    /font-family\s*:\s*([^;}]+)|fontFamily\s*:\s*(?:["']([^"']+)["']|`([^`]+)`)/g,
  )) {
    for (const family of (declared ?? quoted ?? templated ?? "").split(",")) {
      const name = family.trim().replace(/^["'`]|["'`]$/g, "");
      // Three spellings name no family this scan can judge: a `var()` token
      // reference, a family assembled at run time, and `inherit`, which takes
      // whatever the root resolved. Each is checkable where its value comes
      // from, and reporting one here would be a finding about nothing.
      if (name === "" || name === "inherit") continue;
      if (name.startsWith("var(") || name.includes("${")) continue;
      found.push(name);
    }
  }
  return found;
}

describe("design-system conformance gates (B-EP09.1)", scanBudget, () => {
  it("uses only the three §2 type families", () => {
    for (const file of files) {
      // `*.test.*` is fixture data, exactly as the colour arm below reads it
      // and as scripts/check-font-lock.sh already excludes it: a test naming a
      // forbidden family is the assertion, not the defect.
      if (/\.test\.tsx?$/.test(file)) {
        continue;
      }
      // Through `scannableSource`, the same as the colour arm: prose naming a
      // family is discussing the rule, not breaking it, and a detector that
      // strips comments for one rule and not the other is the inconsistency
      // that made this pair hard to reason about.
      for (const name of familiesIn(
        scannableSource(file, sourceTextAt(file)),
      )) {
        expect(
          allowedFamilies.has(name),
          `${relative(frontendRoot, file)}: font-family "${name}" is outside the three-family rule (§2)`,
        ).toBe(true);
      }
    }
  });

  // What the scan above can SEE, planted rather than assumed. Each spelling is
  // one a real file uses, and a miss in any of them reads as a clean PASS.
  it("reads a family in every spelling a source can carry", () => {
    expect(familiesIn('font-family: "Comic Sans MS";')).toEqual([
      "Comic Sans MS",
    ]);
    expect(familiesIn("font-family: Comic Sans MS;")).toEqual([
      "Comic Sans MS",
    ]);
    expect(familiesIn('fontFamily: "Comic Sans MS"')).toEqual([
      "Comic Sans MS",
    ]);
    expect(familiesIn("fontFamily: 'Comic Sans MS'")).toEqual([
      "Comic Sans MS",
    ]);
    expect(familiesIn("fontFamily: `Comic Sans MS`")).toEqual([
      "Comic Sans MS",
    ]);
    // A stack names several, and every one of them is held to the rule.
    expect(familiesIn('font-family: Outfit, "DM Sans", sans-serif;')).toEqual([
      "Outfit",
      "DM Sans",
      "sans-serif",
    ]);
    // The spellings that name nothing come back EMPTY: the allowed token
    // reference, a family built at run time, and the `inherit` the UA reset
    // writes on every control so the root's family reaches it.
    expect(familiesIn("font-family: var(--fontFamilyBody);")).toEqual([]);
    expect(familiesIn("fontFamily: `${chosenFamily}`")).toEqual([]);
    expect(familiesIn("font-family: inherit;")).toEqual([]);
    expect(familiesIn("fontFamily: `inherit`")).toEqual([]);
  });

  // B-EP09.16: no inline user-facing copy — every string the user reads comes
  // from the i18n catalogs. The walk covers JSX text nodes and the attributes
  // that reach the user (aria-label, title, placeholder, alt); fixture data
  // passed as props and non-alphabetic glyphs are not copy.
  it("has no hard-coded user-facing copy outside the i18n catalogs", () => {
    const userFacingAttrs = new Set([
      "aria-label",
      "title",
      "placeholder",
      "alt",
    ]);
    const hasWords = (text: string) => /[A-Za-z]{2,}/.test(text);
    const violations: string[] = [];

    for (const file of files) {
      // Stories (like tests) are catalog fixtures, not shipped UI: their demo
      // copy is deliberately literal — they still stay subject to the emoji and
      // colour-purity checks below, only this i18n-copy rule exempts them.
      //
      // Plus exactly ONE component, named rather than pattern-matched:
      // mcp-apps/story-hosts.tsx is Storybook-only scaffolding whose single
      // string tells a DEVELOPER to run `pnpm build` before the document story
      // can render. No user ever reads it, and it cannot carry a .stories.tsx
      // name because Storybook would then load it as a story module and fail on
      // the component exports. Keep this a NAMED file — widening it to a pattern
      // is how real drift gets in beside it.
      if (
        !file.endsWith(".tsx") ||
        /\.test\.tsx$/.test(file) ||
        /\.stories\.tsx$/.test(file) ||
        file.endsWith("mcp-apps/story-hosts.tsx")
      ) {
        continue;
      }
      const source = sourceFileAt(file);
      const visit = (node: ts.Node) => {
        if (ts.isJsxText(node) && hasWords(node.text)) {
          const { line } = source.getLineAndCharacterOfPosition(
            node.getStart(),
          );
          violations.push(
            `${relative(frontendRoot, file)}:${line + 1} JSX text "${node.text.trim()}"`,
          );
        }
        if (
          ts.isJsxAttribute(node) &&
          userFacingAttrs.has(node.name.getText()) &&
          node.initializer &&
          ts.isStringLiteral(node.initializer) &&
          hasWords(node.initializer.text)
        ) {
          const { line } = source.getLineAndCharacterOfPosition(
            node.getStart(),
          );
          violations.push(
            `${relative(frontendRoot, file)}:${line + 1} ${node.name.getText()}="${node.initializer.text}"`,
          );
        }
        ts.forEachChild(node, visit);
      };
      visit(source);
    }
    expect(violations, violations.join("\n")).toEqual([]);
  });

  // A glyph is a lucide or Material icon. The 🟢/🟡 autonomy marks are the
  // `.dot` token component, so no emoji is ever needed in source text.
  it("uses no emoji glyphs in source strings (§2b)", () => {
    const emoji = /[\u{1F300}-\u{1FAFF}\u{2600}-\u{27BF}]/u;
    const violations: string[] = [];
    for (const file of files) {
      if (
        !/\.(tsx|ts)$/.test(file) ||
        /\.test\.tsx?$/.test(file) ||
        file.endsWith(".d.ts")
      ) {
        continue;
      }
      const source = sourceFileAt(file);
      const visit = (node: ts.Node) => {
        const isText =
          ts.isStringLiteral(node) ||
          ts.isNoSubstitutionTemplateLiteral(node) ||
          ts.isJsxText(node);
        if (isText && emoji.test(node.text)) {
          violations.push(
            `${relative(frontendRoot, file)}: "${node.text.trim()}"`,
          );
        }
        ts.forEachChild(node, visit);
      };
      visit(source);
    }
    expect(violations, violations.join("\n")).toEqual([]);
  });

  it("the web-app manifest is valid and complete for installability", () => {
    const manifest = JSON.parse(
      readFileSync(
        join(frontendRoot, "public", "manifest.webmanifest"),
        "utf8",
      ),
    );
    expect(manifest.name).toBe("Margince");
    expect(manifest.start_url).toBe("/");
    expect(manifest.display).toBe("standalone");
    expect(manifest.icons.length).toBeGreaterThanOrEqual(1);
  });

  // One spelling of the button. `Button` (design-system/button.tsx) is what
  // emits `btn`. A `className` that spells the base class itself is a
  // hand-rolled copy of it, and a copy is frozen at the day it was written.
  // The width floor, the focus ring, the icon sizing and the shared control
  // height all landed on Button. They reached none of the ten copies this gate
  // was written to clear.
  //
  // The rule is deliberately narrow so it states its own exception. It matches
  // the `btn` BASE token only — a `.btn-*` modifier in a STYLESHEET is how the
  // variants are declared, and a component class that merely ends in `btn`
  // (`iconbtn`) is a different control. And it matches every element
  // except an anchor. `Button` renders a `<button>`, so a link that looks like
  // a button (features/settings/client.tsx's "create a lead" href) has no component to
  // reach for. It, and the anchor an `OffsiteLink` draws, is styled by hand.
  it("renders every button through Button — no hand-rolled btn classes", () => {
    const violations: string[] = [];
    for (const file of files) {
      // button.tsx is Button's own file: it is where `btn` is minted.
      if (!file.endsWith(".tsx") || file.endsWith("design-system/button.tsx")) {
        continue;
      }
      const source = sourceFileAt(file);
      const visit = (node: ts.Node) => {
        if (ts.isJsxAttribute(node) && node.name.getText() === "className") {
          const element = node.parent.parent;
          const tag = element.tagName.getText();
          // Every literal fragment the className can evaluate to, so a
          // conditional or an interpolated class list is read too.
          const fragments: string[] = [];
          const collect = (child: ts.Node) => {
            if (
              ts.isStringLiteral(child) ||
              ts.isTemplateLiteralToken(child) ||
              ts.isJsxText(child)
            ) {
              fragments.push(child.text);
            }
            ts.forEachChild(child, collect);
          };
          if (node.initializer) {
            collect(node.initializer);
          }
          const handRolled = fragments.some((fragment) =>
            fragment.split(/\s+/).includes("btn"),
          );
          if (
            handRolled &&
            tag !== "a" &&
            tag !== "OffsiteLink" &&
            tag !== "AppLink"
          ) {
            const { line } = source.getLineAndCharacterOfPosition(
              node.getStart(),
            );
            violations.push(
              `${relative(frontendRoot, file)}:${line + 1} <${tag}> spells the btn class by hand — import Button from design-system/atoms`,
            );
          }
        }
        ts.forEachChild(node, visit);
      };
      visit(source);
    }
    expect(violations, violations.join("\n")).toEqual([]);
  });

  // The card equivalent of the button rule above, and it exists for the same
  // reason: `Card` owns five chrome values — elevated ground, a subtle border,
  // the 12px radius, one padding, and the inset variant — and a surface that
  // spells `card` by hand keeps whichever of the five were true the day it was
  // written. Thirteen sites had drifted that way across the public booking page,
  // the extension client, the preference centre, the OAuth consent screen, Brief
  // and one of two adjacent skeletons on the company record — where the OTHER
  // skeleton, forty lines up, was a real Card.
  //
  // Narrow, so it states its own exception. It matches the `card` and
  // `card-inset` BASE tokens only: a component class that merely contains the
  // word (`auth-card`, `staging-card`, `digest-card`, `co-card`)
  // is a different surface, exactly as `iconbtn` is a different control. And it
  // spares an element that declares a role `Card` cannot express: the component
  // admits `role="status"` and nothing else, on purpose — a card must not be
  // able to claim it is a modal — so a surface that has to announce itself as a
  // `dialog` or a `note` has no component to reach for. Such a surface says so
  // in-source where it does it. The exemption reads the role's LITERAL value
  // and compares it exactly to `status`. A role the source computes
  // (`role={role}`) is NOT an exemption: the gate cannot know what it evaluates
  // to, so it asks rather than assumes — an unreadable role that waved the card
  // through would be the one surface nobody was checking.
  it("renders every card through Card — no hand-rolled card classes", () => {
    const violations: string[] = [];
    for (const file of files) {
      // atoms.tsx is Card's own file: it is where `card` is minted.
      if (!file.endsWith(".tsx") || file.endsWith("design-system/atoms.tsx")) {
        continue;
      }
      const source = sourceFileAt(file);
      const visit = (node: ts.Node) => {
        if (ts.isJsxAttribute(node) && node.name.getText() === "className") {
          const element = node.parent.parent;
          const tag = element.tagName.getText();
          const fragments: string[] = [];
          const collect = (child: ts.Node) => {
            if (
              ts.isStringLiteral(child) ||
              ts.isTemplateLiteralToken(child) ||
              ts.isJsxText(child)
            ) {
              fragments.push(child.text);
            }
            ts.forEachChild(child, collect);
          };
          if (node.initializer) {
            collect(node.initializer);
          }
          const handRolled = fragments.some((fragment) => {
            const tokens = fragment.split(/\s+/);
            return tokens.includes("card") || tokens.includes("card-inset");
          });
          const declaresOtherRole = node.parent.properties.some((property) => {
            if (
              !ts.isJsxAttribute(property) ||
              property.name.getText() !== "role" ||
              property.initializer === undefined
            ) {
              return false;
            }
            const role = literalAttributeValue(property.initializer);
            return role !== undefined && role !== "status";
          });
          if (handRolled && !declaresOtherRole) {
            const { line } = source.getLineAndCharacterOfPosition(
              node.getStart(),
            );
            violations.push(
              `${relative(frontendRoot, file)}:${line + 1} <${tag}> spells the card class by hand — import Card from design-system/atoms`,
            );
          }
        }
        ts.forEachChild(node, visit);
      };
      visit(source);
    }
    expect(violations, violations.join("\n")).toEqual([]);
  });

  it("keeps literal colours in tokens.css only — everything else reads a token", () => {
    // Asserted once: half a million `expect` calls cost more than the scan.
    const hardCoded: string[] = [];
    for (const file of files) {
      // tokens.css is where literals live (tests pin them); index.html's
      // meta theme-color cannot read a CSS custom property.
      //
      // Two NAMED files, not a widened pattern, both repeated in
      // scripts/check-ds-purity.sh so neither arm can be satisfied alone.
      // provider-mark.tsx carries another company's sign-in marks, which are
      // not ours to tokenise. tokens-testing.ts is what the token suites READ
      // the sheet with: it declares no colour, and `rgba(` there is the grammar
      // it parses rather than paint it applies.
      if (
        file.endsWith("tokens.css") ||
        file.endsWith("index.html") ||
        file.endsWith("provider-mark.tsx") ||
        /\.test\.tsx?$|tokens-testing\.ts$/.test(file)
      ) {
        continue;
      }
      const text = scannableSource(file, sourceTextAt(file));
      for (const [index, line] of text.split("\n").entries()) {
        if (LITERAL_COLOUR.test(line)) {
          hardCoded.push(`${relative(frontendRoot, file)}:${index + 1}`);
        }
      }
    }
    expect(
      hardCoded,
      `these lines hard-code a colour — read it from a token:\n${hardCoded.join("\n")}`,
    ).toEqual([]);
  });

  it("reads a colour beside a comment, and not an issue reference inside one", () => {
    const sample = [
      'import "./x.css";',
      "// the app-wide version lands with #2455",
      "/* and #2456 with it */",
      "export function Swatch() {",
      "  return (",
      '    <div className="t-caption">',
      "      {/* ... see #2463 for the app-wide version. */}",
      '      <span style={{ color: "#ff0000" }} />',
      "    </div>",
      "  );",
      "}",
    ].join("\n");
    const flagged = scannableSource("sample.tsx", sample)
      .split("\n")
      .flatMap((line, index) => (LITERAL_COLOUR.test(line) ? [index + 1] : []));
    // Only the style attribute. Every other `#NNNN` above is an issue number
    // sitting in a comment — and the gate must still SEE the literal on the
    // line under the JSX one, or it has bought its silence by going blind.
    expect(flagged).toEqual([8]);
  });

  // The same rule in the languages the parser arm never sees. A stylesheet is
  // where colours actually live, so a gate that reads its comments names the
  // wrong line most often exactly where it matters most.
  it("reads past a CSS comment without going blind to the rule beside it", () => {
    const sample = [
      '/* see #2463, and font-family: "Comic Sans MS" is discussed there */',
      ".swatch {",
      "  color: #ff0000;",
      "}",
    ].join("\n");
    const scanned = scannableSource("sample.css", sample);
    const flagged = scanned
      .split("\n")
      .flatMap((line, index) => (LITERAL_COLOUR.test(line) ? [index + 1] : []));
    // Only the declaration. The comment names an issue and a family, and
    // neither is something the file ships.
    expect(flagged).toEqual([3]);
    expect(familiesIn(scanned)).toEqual([]);
  });

  it("reads past an HTML comment the same way", () => {
    const sample = [
      "<!-- #2463: the meta colour cannot read a custom property -->",
      '<meta name="theme-color" content="#0b1f17" />',
    ].join("\n");
    const flagged = scannableSource("sample.html", sample)
      .split("\n")
      .flatMap((line, index) => (LITERAL_COLOUR.test(line) ? [index + 1] : []));
    expect(flagged).toEqual([2]);
  });
});
