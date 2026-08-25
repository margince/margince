import { readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import {
  declarations,
  rules,
  soleRule,
  stylesheets,
  withoutComments,
} from "./cssrules";

// `.t-eyebrow` is the one spelling of uppercase micro-type, and this is what
// makes that a fact rather than a retelling.
//
// Its own docblock in base.css has said "THE one spelling" since it was
// written, and said it over TWENTY-SEVEN rules outside the design system that
// re-declared the identical quadruple — the eyebrow font size, the semibold
// weight, the eyebrow tracking and the uppercase transform — across the
// onboarding screens, the conversation sheet and the workbench chrome itself.
// Every copy looked right on its own. That is the shape this repository's
// rulebook forbids: a uniqueness claim no test holds is either deleted or
// gated.
//
// The four are ONE decision, not four properties. Uppercase at 11px sets
// cramped at default spacing, which is what --tracking-eyebrow exists for, and
// a copy that took three of the four had already made the type worse without
// anybody choosing it.
//
// WHAT IT DELIBERATELY DOES NOT CATCH. A rule that takes SOME of the quadruple
// is a variant somebody decided on, not a duplicate a test can name — an
// uppercase label at a different size is a design decision, and reporting it
// would be this gate asserting an answer nobody gave. It asks the narrow
// question it can answer: is this the eyebrow, copied.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const designSystem = join(frontendRoot, "src", "design-system");
// The one rule this gate reads its subject FROM, and so the one rule it must
// not report as a copy of itself.
const eyebrowSource = join(designSystem, "base.css");
const tokens = join(designSystem, "tokens.css");

/**
 * `.t-eyebrow`'s own type declarations, read from the design system rather
 * than repeated here. A hardcoded copy of the quadruple would be the very thing
 * this forbids, and would stop matching the day somebody restyles the eyebrow.
 *
 * `color` is dropped on purpose: it is the one property a caller is expected to
 * override — the tree has eyebrows in accent, in AI text and in three meta
 * roles — so requiring it would make the gate see almost nothing.
 */
function eyebrowType(): readonly string[] {
  return declarations(soleRule(eyebrowSource, ".t-eyebrow").body)
    .filter((declaration) => !declaration.startsWith("color:"))
    .map(resolved);
}

/**
 * A declaration with each token reference replaced by the token's value.
 *
 * The reference is not spelled out in this sentence on purpose: the
 * custom-property gate reads every `var(…)` in the tree, prose included, and a
 * made-up token name in an explanation is reported as one nothing defines.
 *
 * Both sides of the comparison run through this, because `font-weight: 600` and
 * `font-weight: var(--fw-semibold)` are the same weight and compare unequal —
 * and the longhand is exactly what the hand that retypes a rule writes. A text
 * comparison loses to an equivalent spelling, every time.
 */
function resolved(declaration: string): string {
  return declaration.replace(/var\((--[\w-]+)\)/g, (whole, name: string) => {
    const value = tokenValues().get(name);
    return value ?? whole;
  });
}

let cachedTokens: Map<string, string> | null = null;
function tokenValues(): Map<string, string> {
  if (cachedTokens !== null) {
    return cachedTokens;
  }
  const values = new Map<string, string>();
  for (const rule of rules(withoutComments(readFileSync(tokens, "utf8")))) {
    for (const declaration of declarations(rule.body)) {
      const colon = declaration.indexOf(": ");
      const property = declaration.slice(0, colon);
      if (property.startsWith("--")) {
        values.set(property, declaration.slice(colon + 2));
      }
    }
  }
  cachedTokens = values;
  return values;
}

describe("one eyebrow", () => {
  const sheets = stylesheets(join(frontendRoot, "src"));

  it("finds stylesheets to judge", () => {
    // A census that read nothing certifies nothing, and this one is a census of
    // zero once the tree is clean — it reads the same over a clean tree and a
    // broken walk. So it also names the two places the copies actually lived.
    expect(sheets.length).toBeGreaterThan(40);
    expect(
      sheets.some((path) =>
        path.endsWith("onboarding-conversation/conversation.css"),
      ),
      "the walk did not reach the conversation sheet, which carried twelve of the twenty-seven copies",
    ).toBe(true);
    expect(
      sheets.some((path) =>
        path.endsWith("design-system/margince-workbench.css"),
      ),
      "the walk did not reach the workbench chrome, where five copies sat inside the design system itself",
    ).toBe(true);
  });

  it("reads a subject worth comparing", () => {
    // If the source rule ever stops declaring the quadruple, every rule in the
    // tree trivially "contains" an empty subject and the gate would report the
    // whole stylesheet — or, with one declaration left, almost nothing. Naming
    // the count is what tells a restyle from a deletion.
    expect(eyebrowType()).toHaveLength(4);
  });

  it("is declared only by the design system", () => {
    const type = eyebrowType();
    const copies: string[] = [];
    for (const path of sheets) {
      for (const rule of rules(withoutComments(readFileSync(path, "utf8")))) {
        if (path === eyebrowSource && rule.selector === ".t-eyebrow") {
          continue;
        }
        const declared = new Set(declarations(rule.body).map(resolved));
        if (type.every((property) => declared.has(property))) {
          copies.push(`${relative(frontendRoot, path)}: ${rule.selector}`);
        }
      }
    }
    expect(
      copies,
      "these rules re-declare the eyebrow. Carry `t-eyebrow` in the markup and " +
        "keep only what is this element's own — its colour, its spacing, its " +
        "layout. The four are one decision: uppercase at 11px sets cramped " +
        "without --tracking-eyebrow, and a copy that took three of the four " +
        "made the type worse without anybody choosing it.",
    ).toEqual([]);
  });
});
