/** @vitest-environment jsdom */
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { PanelRow } from "./panel";

afterEach(cleanup);

const here = dirname(fileURLToPath(import.meta.url));

function panelCss(): string {
  return readFileSync(join(here, "panel.css"), "utf8");
}

function tokensCss(): string {
  return readFileSync(join(here, "tokens.css"), "utf8");
}

// The rule and the hover are two shapes, and PanelRow used to hold them
// together: every row lit up under the pointer, so a panel of ruled blocks a
// reader is meant to READ told them all five were pressable. The default is
// therefore inert, and a caller opts in only when the whole row is one press
// target.
describe("PanelRow separates the hairline from the press", () => {
  it("draws an inert row by default", () => {
    const { container } = render(<PanelRow>Renewal date</PanelRow>);
    const row = container.querySelector(".panel-row");
    expect(row).not.toBeNull();
    expect(row?.classList.contains("panel-row-interactive")).toBe(false);
  });

  it("marks the row a press target when the caller says it is one", () => {
    const { container } = render(
      <PanelRow interactive>
        <button type="button">Q4 — renewals</button>
      </PanelRow>,
    );
    expect(
      container
        .querySelector(".panel-row")
        ?.classList.contains("panel-row-interactive"),
    ).toBe(true);
  });

  // The caller's own class survives the variant: a screen names its row for
  // layout, and the two spellings have to coexist on one element.
  it("keeps the caller's class beside the variant", () => {
    const { container } = render(
      <PanelRow interactive className="panel-row-on">
        Q3
      </PanelRow>,
    );
    const row = container.querySelector(".panel-row");
    expect(row?.classList.contains("panel-row-interactive")).toBe(true);
    expect(row?.classList.contains("panel-row-on")).toBe(true);
  });
});

// A deleted rule body still parses and still paints, so the stylesheet is
// asserted on directly rather than trusting the class list above: the hover
// fill has to exist, and it has to hang on the interactive class alone.
describe("panel.css keeps the row's hover on the interactive variant", () => {
  it("declares the hover fill only for an interactive row", () => {
    const css = panelCss();
    const hovers = [...css.matchAll(/([^{}]*:hover)\s*\{([^}]*)\}/g)].filter(
      ([, selector]) => selector.includes(".panel-row"),
    );
    expect(hovers.length).toBe(1);
    const [selector, body] = [hovers[0][1].trim(), hovers[0][2]];
    expect(selector).toBe(".panel-row-interactive:hover");
    // The body is the point of the variant. An emptied rule reads as a live
    // one to every gate that only counts selectors.
    expect(body).toMatch(/background:\s*var\(--bgHover\)/);
  });

  it("leaves the bare row its hairline and nothing that suggests a press", () => {
    const bare = /(?:^|\n)\.panel-row\s*\{([^}]*)\}/.exec(panelCss());
    expect(bare).not.toBeNull();
    const body = bare?.[1] ?? "";
    expect(body).not.toMatch(/background/);
    // A transition on a row with no state to move between is the leftover of
    // the hover it used to carry.
    expect(body).not.toMatch(/transition/);

    // The hairline is drawn as an inset pseudo-element rather than a border,
    // because a border cannot stop at the card's padding — every rule BETWEEN
    // two pieces of a card's content does, and only the header's and footer's
    // run edge to edge. Asserted on the rule that draws it, so an inset that
    // gets dropped back onto the row's own border still fails here.
    const line = /(?:^|\n)\.panel-row::before\s*\{([^}]*)\}/.exec(panelCss());
    expect(line).not.toBeNull();
    const drawn = line?.[1] ?? "";
    expect(drawn).toMatch(/background:\s*var\(--borderSubtle\)/);
    expect(drawn).toMatch(/height:\s*1px/);
    // WHICH inset: the panel's own, read off the body rather than spelled here.
    // The hairline stopping at the padding is the whole point of drawing it as
    // a pseudo-element, so a body retuned to a different token while the line
    // kept the old one would leave every rule in the panel a few pixels short
    // of the text above it — and a literal expectation here would still pass.
    const bodyRule = /(?:^|\n)\.panel-body\s*\{([^}]*)\}/.exec(panelCss());
    const inset = /padding:\s*(var\(--[a-zA-Z0-9-]+\))/.exec(
      bodyRule?.[1] ?? "",
    );
    expect(inset).not.toBeNull();
    expect(drawn).toContain(`inset: 0 ${inset?.[1]} auto`);
  });

  // The card's own chrome keeps its full-width border: the header and the footer
  // divide the card FROM its content, and every card in the product draws that
  // band the same way. Held here because the inset sweep above went through them
  // once, and a page of cards whose headers stop short of the edge reads as a
  // different card from every other surface.
  it("rules the header and the footer edge to edge", () => {
    const css = panelCss();
    const head = /(?:^|\n)\.panel-head\s*\{([^}]*)\}/.exec(css)?.[1] ?? "";
    expect(head).toMatch(/border-bottom:\s*1px solid var\(--borderSubtle\)/);
    const foot = /(?:^|\n)\.panel-foot\s*\{([^}]*)\}/.exec(css)?.[1] ?? "";
    expect(foot).toMatch(/border-top:\s*1px solid var\(--borderSubtle\)/);
    expect(css).not.toMatch(/\.panel-head::after/);
    expect(css).not.toMatch(/\.panel-foot::before/);
  });
});

// ---------------------------------------------------------------------------
// The head band: one height, one owner.
//
// The rules below read stylesheets rather than a rendered box because jsdom
// lays nothing out — and because the invariant is about the DECLARATIONS. A
// band that measures 56px on the one screen a render test happens to mount says
// nothing about the screen that re-spaced it in a sheet of its own, which is
// exactly how the same card came to stand at three heights.
// ---------------------------------------------------------------------------

// One rule as the sweeps below read it: what it styles, and what it declares.
type CssRule = Readonly<{
  selector: string;
  block: string;
  properties: readonly string[];
}>;

// The band's own geometry — how tall it is, the air inside it, how its content
// lines up. Whoever sets one of these decides the height of every panel head on
// the screen, and that decision belongs to panel.css alone.
const BAND_GEOMETRY =
  /^(?:height|min-height|max-height|padding(?:-top|-bottom|-block|-block-start|-block-end)?|align-items|flex-wrap)$/;
// The band's lower edge is the card's own chrome, drawn edge to edge on every
// panel in the product. A tone recolours that hairline — tint on the same
// geometry, held below — but no sheet outside panel.css redraws or drops it.
const BAND_EDGE = /^border-bottom(?:-|$)/;

function stripComments(css: string): string {
  return css.replace(/\/\*[\s\S]*?\*\//g, "");
}

// An at-rule that opens a block (@media, @container, @supports) WRAPS ordinary
// rules, so dropping its prelude and brace leaves those rules at the top level
// where the scan below reads them; the orphaned closing brace matches no
// selector. A statement at-rule (@import) opens no block and is dropped whole,
// as far as its semicolon and no further: consuming to the next brace instead
// would take the selector after it along with it, and a sweep that reads a
// smaller tree reports PASS with nothing to notice.
function unwrapAtRules(css: string): string {
  return css.replace(/@[a-zA-Z-]+[^{};]*[{;]/g, "");
}

function declaredProperties(block: string): readonly string[] {
  return block
    .split(";")
    .map((declaration) => declaration.split(":")[0].trim().toLowerCase())
    .filter((property) => /^[a-z-]+$/.test(property));
}

function cssRules(css: string): readonly CssRule[] {
  const flat = unwrapAtRules(stripComments(css));
  return [...flat.matchAll(/([^{}]+)\{([^{}]*)\}/g)].flatMap(
    ([, selectorList, block]) =>
      selectorList.split(",").map((selector) => ({
        // One space per combinator, whatever the sheet wrapped across lines:
        // the compound scan below reads a descendant combinator as a space.
        selector: selector.trim().replace(/\s+/g, " "),
        block,
        properties: declaredProperties(block),
      })),
  );
}

// What a rule STYLES is the last compound of its selector: `.pe-memory
// .panel-head` re-shapes the band, `.panel-head .panel-title` shapes the title
// inside it, `.panel-head > .ext-unit-actions` an action beside it. Combinators
// inside parentheses do not divide a compound, so `:has(.panel-head-sub)` stays
// part of the band it qualifies.
function lastCompound(selector: string): string {
  let depth = 0;
  let start = 0;
  for (let index = 0; index < selector.length; index += 1) {
    const character = selector[index];
    if (character === "(") depth += 1;
    else if (character === ")") depth -= 1;
    else if (depth === 0 && " >+~".includes(character)) start = index + 1;
  }
  return selector.slice(start);
}

// `.panel-head-text` and `.panel-head-sub` open with the same eleven characters
// and are content, not the band.
function stylesTheBand(selector: string): boolean {
  return /^\.panel-head(?![\w-])/.test(lastCompound(selector));
}

function bandRules(css: string): readonly CssRule[] {
  return cssRules(css).filter((rule) => stylesTheBand(rule.selector));
}

// Comments are stripped first here and in the gap below: prose about a
// property reads exactly like the property to a regex, and a sentence opening
// "No gap: …" is what the stack's own comment says.
function tokenValue(name: string): string {
  const declared = new RegExp(`${name}:\\s*([^;]+);`).exec(
    stripComments(tokensCss()),
  );
  expect(declared, `${name} is declared in tokens.css`).not.toBeNull();
  return (declared?.[1] ?? "").trim();
}

function tokenPixels(name: string): number {
  return Number.parseFloat(tokenValue(name));
}

// The gap the title stack takes, read off the rule rather than restated here:
// a literal expectation would still pass the day somebody gives the stack a
// rung back and pushes the two lines past the band.
function titleStackGap(): number {
  const stack = cssRules(panelCss()).find(
    (rule) => rule.selector === ".panel-head-text",
  );
  const gap = /gap:\s*([^;]+)/.exec(stack?.block ?? "")?.[1].trim() ?? "0";
  const token = /^var\((--[\w-]+)\)$/.exec(gap);
  return token ? tokenPixels(token[1]) : Number.parseFloat(gap);
}

describe("the panel head is one band, fixed at the height every panel shares", () => {
  it("takes its height from the house token rather than a floor of its own", () => {
    const head = bandRules(panelCss()).find(
      (rule) => rule.selector === ".panel-head",
    );
    expect(head).toBeDefined();
    expect(head?.block).toMatch(/height:\s*var\(--panel-head-h\)/);
    // A floor is an invitation: a description raised the band, a screen then
    // re-spaced it, and the same card stood at three heights across one page.
    expect(head?.properties).not.toContain("min-height");
    expect(head?.block).toMatch(/align-items:\s*center/);
    expect(head?.block).toMatch(/flex-wrap:\s*nowrap/);
    expect(head?.block).toMatch(/padding:\s*0 var\(--padPanel\)/);
    expect(tokenValue("--panel-head-h")).toBe("56px");
  });

  // One value at every viewport, like the band it is. A touch arm that raised
  // it would give the same panel two heights on one machine.
  it("keeps the band at one height on every viewport", () => {
    const declarations = tokensCss().match(/--panel-head-h:/g) ?? [];
    expect(declarations).toHaveLength(1);
  });

  it("holds a description inside the band instead of growing for one", () => {
    expect(panelCss()).not.toMatch(/\.panel-head:has\(/);
    // jsdom lays nothing out, so the question the band has to answer — does a
    // title over a description still fit — is arithmetic on the type scale.
    // This fails if the meta rung grows, if the leading is retuned, or if the
    // stack takes a gap back, which are the three ways the pair stops fitting.
    const leading = Number.parseFloat(tokenValue("--lh-normal"));
    const stack =
      tokenPixels("--fs-h3") * leading +
      tokenPixels("--fs-meta") * leading +
      titleStackGap();
    expect(stack).toBeLessThanOrEqual(tokenPixels("--panel-head-h"));
  });

  // Nothing in the band wraps to a second line, because a second line is a
  // second height. The title and the description end in an ellipsis instead,
  // and only they give way: a badge or a button squeezed by a long title reads
  // as a different control.
  it("truncates the two lines and lets nothing else give way", () => {
    const truncated =
      /(?:^|\n)\.panel-head \.panel-title,\n\.panel-head-sub\s*\{([^}]*)\}/.exec(
        panelCss(),
      )?.[1] ?? "";
    expect(truncated).toMatch(/white-space:\s*nowrap/);
    expect(truncated).toMatch(/overflow:\s*hidden/);
    expect(truncated).toMatch(/text-overflow:\s*ellipsis/);
    expect(truncated).toMatch(/min-width:\s*0/);

    const stack = /(?:^|\n)\.panel-head-text\s*\{([^}]*)\}/.exec(panelCss());
    expect(stack?.[1]).toMatch(/min-width:\s*0/);
    // The band itself does not clip: a menu or a tooltip opened from a button
    // in the head has to be able to leave it.
    const head = bandRules(panelCss()).find(
      (rule) => rule.selector === ".panel-head",
    );
    expect(head?.properties).not.toContain("overflow");
  });
});

// A tone is a claim — the ordinary ask, the bad news, a machine wrote this —
// and a claim is made in colour. A panel that changed SIZE with its tone would
// read as a different card rather than as the same card in a different mood,
// which is what the ai head did while it hugged its own two lines.
describe("a panel tone tints the head band and never reshapes it", () => {
  const toned = () =>
    bandRules(panelCss()).filter((rule) =>
      /^\.panel-(?:accent|warn|ai)\b/.test(rule.selector),
    );

  it("leaves the band's geometry to the band", () => {
    expect(toned().length).toBeGreaterThan(0);
    const reshaped = toned().flatMap((rule) =>
      rule.properties
        .filter((property) => BAND_GEOMETRY.test(property))
        .map((property) => `${rule.selector} sets ${property}`),
    );
    expect(reshaped, "a tone may tint the band, not resize it").toEqual([]);
  });

  it("recolours the band's hairline without redrawing it", () => {
    const edges = toned()
      .map((rule) => /border-bottom:\s*([^;]+)/.exec(rule.block)?.[1].trim())
      .filter((edge): edge is string => edge !== undefined);
    expect(edges.length).toBeGreaterThan(0);
    for (const edge of edges) {
      expect(edge).toMatch(/^1px solid var\(--[\w-]+\)$/);
    }
  });
});

// The sweep. `check-ds-spacing-roles.sh` holds the VOCABULARY a screen re-spaces
// a primitive in; this holds that the head band is not a screen's to re-space at
// all, in any vocabulary — a role-spelled `padding-top: var(--padPanel)` passes
// that gate and still gives one screen a taller panel than every other.
describe("panel.css is the only sheet that shapes the head band", () => {
  const src = join(here, "..");
  const extensions = join(here, "..", "..", "..", "extensions");
  const owner = join(here, "panel.css");

  function stylesheetsUnder(root: string): readonly string[] {
    if (!existsSync(root)) return [];
    return readdirSync(root, { withFileTypes: true }).flatMap((entry) => {
      const path = join(root, entry.name);
      if (entry.isDirectory()) {
        return entry.name === "node_modules" ? [] : stylesheetsUnder(path);
      }
      return entry.isFile() && path.endsWith(".css") ? [path] : [];
    });
  }

  // The detector, proven against the one rule that legitimately declares the
  // band: a scan that stopped recognising `.panel-head` would sweep a smaller
  // tree, report PASS, and leave nothing to notice.
  it("recognises the band where it is declared", () => {
    const own = bandRules(panelCss());
    expect(own.map((rule) => rule.selector)).toContain(".panel-head");
  });

  it("reads a rule about the head's CONTENT as content", () => {
    const inside = bandRules(`
      .co-glance-cols .panel > .panel-head .panel-title { font-size: var(--fs-h2); }
      .ext-unit > .panel-head > .ext-unit-actions { flex: 0 1 auto; }
      .panel-head-text { gap: 0; }
      .panel-head-sub { font-size: var(--fs-meta); }
    `);
    expect(inside).toEqual([]);
  });

  it("reads a band rule wrapped in a query, and one that follows an import", () => {
    const wrapped = bandRules(`
      @import "./other.css";
      @media (max-width: 40rem) {
        .pe-memory .panel-head { min-height: 0; }
      }
      .co-glance .panel > .panel-head:has(.panel-title) { padding-block: 0; }
    `);
    expect(wrapped.map((rule) => rule.selector)).toEqual([
      ".pe-memory .panel-head",
      ".co-glance .panel > .panel-head:has(.panel-title)",
    ]);
  });

  it("finds no other sheet setting the band's geometry or its edge", () => {
    const swept = [...stylesheetsUnder(src), ...stylesheetsUnder(extensions)]
      .filter((path) => path !== owner)
      .sort();
    // Named members, not just a count: a walker that silently reached a
    // smaller tree would sweep past the screens that once carried overrides
    // and still report PASS.
    expect(swept.length).toBeGreaterThan(0);
    expect(
      swept.filter(
        (path) =>
          path.endsWith(join("screens", "company", "glance.css")) ||
          path.endsWith(join("screens", "person360.css")),
      ),
    ).toHaveLength(2);

    const offences = swept.flatMap((path) =>
      bandRules(readFileSync(path, "utf8")).flatMap((rule) =>
        rule.properties
          .filter(
            (property) =>
              BAND_GEOMETRY.test(property) || BAND_EDGE.test(property),
          )
          .map(
            (property) =>
              `${relative(src, path)}: ${rule.selector} sets ${property}`,
          ),
      ),
    );
    expect(
      offences,
      "the head band is 56px on every screen: state the difference in panel.css or put the content in the body",
    ).toEqual([]);
  });
});
