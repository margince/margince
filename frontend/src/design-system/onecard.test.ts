import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// `Card` is the one card surface, and a hand-rolled one is a second card the
// moment any of its chrome moves — which is not a warning, it is a description
// of what this tree did. `onboarding.css` grew SIX card classes: `.rcard` was
// byte-identical to `.card`, `.finding-card` dropped the shadow and swapped the
// background, `.legal-preview-card` reached for a stronger border, and
// `.fact-card` / `.legal-card` were the same card twice — identical chrome with
// text rules that had already drifted apart.
//
// So the rule is derived rather than remembered: a rule outside the design
// system that declares `.card`'s OWN chrome — the same four token values, plus
// the card shadow — is a second card, whatever it is called.
//
// WHAT IT DELIBERATELY DOES NOT CATCH, because a gate that fires on correct code
// teaches people to skip its output. "A border, a radius and a padded
// background" describes 64 rules in this tree — chips, search fields, plates,
// callouts — and almost none of them is a card. So this asks the narrow
// question it can answer: is this the card surface, copied. A VARIANT of it
// (`.finding-card` dropped the shadow and swapped the background) is a design
// decision somebody has to make, not a duplicate a test can name, and reporting
// every one would be this gate asserting an answer nobody gave.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const designSystem = join(frontendRoot, "src", "design-system");

function stylesheets(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return entry.name === "node_modules" || entry.name === "dist"
        ? []
        : stylesheets(path);
    }
    return entry.name.endsWith(".css") ? [path] : [];
  });
}

/** One `selector { … }` rule, as written. */
type Rule = Readonly<{ selector: string; body: string }>;

/** CSS comments are not rules, and one sitting above a rule was being read as
 * its selector. */
function withoutComments(css: string): string {
  return css.replace(/\/\*[\s\S]*?\*\//g, "");
}

function rules(css: string): Rule[] {
  const found: Rule[] = [];
  // Non-greedy to the first `}`, which is why a nested block (a media query,
  // `:is()` with braces) is not matched as one rule — the rules INSIDE it are,
  // which is what this asks about anyway.
  for (const match of css.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    found.push({ selector: match[1].trim(), body: match[2] });
  }
  return found;
}

/**
 * `.card`'s own declarations, read from the design system rather than repeated
 * here — a hardcoded copy of the chrome would be the very thing this forbids,
 * and would stop matching the day somebody restyles the real card.
 */
function cardChrome(): readonly string[] {
  const atoms = readFileSync(join(designSystem, "atoms.css"), "utf8");
  const declared = rules(withoutComments(atoms)).filter(
    (r) => r.selector === ".card",
  );
  // EXACTLY one, not the first. `.find()` silently adopts whichever comes
  // first, so a second `.card { … }` anywhere in the file — a media query
  // override, a leftover — becomes the subject and the gate goes looking for
  // the wrong declarations. It failed loudly when I planted one; that was luck,
  // and with a rarer declaration in the first rule it degrades to green while
  // blind.
  if (declared.length !== 1) {
    throw new Error(
      `atoms.css declares .card ${declared.length} times; this gate needs exactly one to read its subject from`,
    );
  }
  return declarations(declared[0].body);
}

/** The `property: value` pairs in a rule body, normalised for whitespace. */
function declarations(body: string): readonly string[] {
  return body
    .split(";")
    .map((part) =>
      part
        .trim()
        .replace(/\s+/g, " ")
        // Around the colon too. Collapsing only runs of whitespace left
        // `background:var(--bgElevated)` unequal to `background: var(...)`, so
        // the same card written without a space escaped entirely — not a
        // variant, the identical card, defeated by a spelling a formatter would
        // erase. This gate exists for the hand that does not run the formatter.
        .replace(/\s*:\s*/g, ": "),
    )
    .filter((part) => part.length > 0);
}

describe("one card surface", () => {
  const outside = stylesheets(join(frontendRoot, "src")).filter(
    (path) => !path.startsWith(designSystem),
  );

  it("finds stylesheets to judge", () => {
    // A census that read nothing certifies nothing, and this one is a census of
    // zero once the tree is clean — it reads the same over a clean tree and a
    // broken walk. The floor was 5 against a real 80, which a walk reaching only
    // the top level and `src/app/` clears with every screen unvisited, so it
    // also names a file it must have reached.
    expect(outside.length).toBeGreaterThan(40);
    expect(
      outside.some((path) => path.endsWith("screens/onboarding.css")),
      "the walk did not reach src/screens/, where every card this gate was written for lived",
    ).toBe(true);
  });

  it("is drawn only by the design system", () => {
    const chrome = cardChrome();
    const second: string[] = [];
    for (const path of outside) {
      for (const rule of rules(withoutComments(readFileSync(path, "utf8")))) {
        const declared = new Set(declarations(rule.body));
        if (chrome.every((property) => declared.has(property))) {
          second.push(`${relative(frontendRoot, path)}: ${rule.selector}`);
        }
      }
    }
    expect(
      second,
      "these rules draw a card surface outside the design system. `Card` is the " +
        "one card — `as` picks the element, `className` still carries whatever " +
        "content styling the screen owns. A copy drifts silently: the two cards " +
        "this replaced had identical chrome and text rules that had already " +
        "disagreed, and nobody chose the difference.",
    ).toEqual([]);
  });
});
