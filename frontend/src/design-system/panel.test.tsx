/** @vitest-environment happy-dom */
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import {
  BAND_GEOMETRY,
  bandRules,
  cssRules,
  declaredValue,
  stateArmRoleInsets,
  stripComments,
  titleRules,
  titleTypeRules,
} from "../../census/rules/panel.census";
import { stable } from "../format/collate";
import { LocaleProvider } from "../i18n";
import { EmptyState } from "./atoms";
import { PANEL_TONES, Panel, PanelBody, PanelIntro } from "./panel";

afterEach(cleanup);

const here = dirname(fileURLToPath(import.meta.url));

function panelCss(): string {
  return readFileSync(join(here, "panel.css"), "utf8");
}

function tokensCss(): string {
  return readFileSync(join(here, "tokens.css"), "utf8");
}

// The "?" beside a title explains the section to a reader who has never met
// it, by pointer and by keyboard alike.
describe("a panel's help", () => {
  it("shows what the section is for on focus, and hides it on Escape", async () => {
    render(
      <LocaleProvider initial="en">
        <Panel title="Growth fit" help="How well this company fits.">
          <PanelBody>Strong fit</PanelBody>
        </Panel>
      </LocaleProvider>,
    );
    const region = screen.getByRole("region", { name: "Growth fit" });
    const mark = within(region).getByRole("button", {
      name: "About this section",
    });
    // The title still names the region alone.
    expect(within(region).getByRole("heading").textContent).toBe("Growth fit");

    await userEvent.tab();
    expect(document.activeElement).toBe(mark);
    expect(screen.getByRole("tooltip").textContent).toContain(
      "How well this company fits.",
    );

    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("draws no mark without help", () => {
    render(
      <Panel title="Consent">
        <PanelBody>Outbound is default-deny per purpose.</PanelBody>
      </Panel>,
    );
    expect(screen.queryByRole("button")).toBeNull();
  });
});

// A page of zones is a page of landmarks, and the title is what names each of
// them: a reader jumping by region hears the same word a sighted reader scans
// the column for. A `<section>` carries no role until it has an accessible
// name, so an unnamed panel was not in that list at all.
describe("a titled panel is a region named by its title", () => {
  it("names the region with the heading the panel already draws", () => {
    render(
      <Panel title="Consent">
        <PanelBody>Outbound is default-deny per purpose.</PanelBody>
      </Panel>,
    );
    const region = screen.getByRole("region", { name: "Consent" });
    // Named BY the heading, not by a copy of it: the id has to point at the
    // element the reader sees, or the two can be edited apart.
    expect(
      within(region).getByRole("heading", { name: "Consent", level: 2 }),
    ).toBe(
      document.getElementById(region.getAttribute("aria-labelledby") ?? ""),
    );
  });

  it("names it at level 3 when the caller sits under a dialog's own title", () => {
    render(
      <Panel title="Rooms" titleLevel={3}>
        <PanelBody>None yet</PanelBody>
      </Panel>,
    );
    const region = screen.getByRole("region", { name: "Rooms" });
    expect(
      within(region).getByRole("heading", { name: "Rooms", level: 3 }),
    ).toBeTruthy();
  });

  // Two panels on one page are two distinct landmarks. A hard-coded id would
  // point both at the first heading and speak one name twice.
  it("gives each panel on a page its own name", () => {
    render(
      <>
        <Panel title="Consent">
          <PanelBody>a</PanelBody>
        </Panel>
        <Panel title="Identity">
          <PanelBody>b</PanelBody>
        </Panel>
      </>,
    );
    expect(screen.getByRole("region", { name: "Consent" })).not.toBe(
      screen.getByRole("region", { name: "Identity" }),
    );
  });

  // An untitled panel is a container the caller chose not to name, and a
  // nameless region in a reader's landmark list is worse than no entry.
  it("claims no landmark when there is no title to name it", () => {
    const { container } = render(
      <Panel>
        <PanelBody>Rows with no head above them</PanelBody>
      </Panel>,
    );
    expect(screen.queryByRole("region")).toBeNull();
    expect(
      container.querySelector(".panel")?.hasAttribute("aria-labelledby"),
    ).toBe(false);
  });
});

// A deleted rule body still parses and still paints, so the stylesheet is
// asserted on directly rather than trusting the class list in
// panel.rows.test.tsx. The hover fill has to exist, and it has to hang on the
// interactive class alone.
describe("panel.css keeps the row's hover on its two variants", () => {
  it("declares the hover fill only for an interactive row and a record row", () => {
    const css = stripComments(panelCss());
    const hovers = [...css.matchAll(/([^{}]*:hover)\s*\{([^}]*)\}/g)].filter(
      ([, selector]) => selector.includes(".panel-row"),
    );
    expect(hovers.map(([, selector]) => selector.trim())).toEqual([
      ".panel-row-interactive:hover",
      ".panel-row-record:hover",
    ]);
    // The body is the point of the variant. An emptied rule reads as a live
    // one to every gate that only counts selectors.
    for (const [, , body] of hovers) {
      expect(body).toMatch(/background:\s*var\(--groundHover\)/);
    }
  });

  // A record list reads as a table, so its rows' rules reach the pane's edges.
  // Every seam between two bands does too, whatever band stands above it.
  it("runs a record row's hairline and every seam edge to edge", () => {
    const rules = cssRules(panelCss());
    const insetOf = (selector: string) =>
      rules
        .filter((rule) => rule.selector === selector)
        .map((rule) => declaredValue(rule.block, "inset"))
        .filter((value) => value !== undefined)
        .at(-1);
    expect(insetOf(".panel-row-record::before")).toBe("0 0 auto");
    expect(insetOf(".panel > :not(.panel-row) + .panel-row::before")).toBe(
      "0 0 auto",
    );
    const seams = rules.filter((rule) =>
      / \+ \.panel-body::before$/.test(rule.selector),
    );
    expect(seams.length).toBeGreaterThanOrEqual(4);
    for (const seam of seams) {
      const content = declaredValue(seam.block, "content");
      if (content === "none") continue;
      expect(content, seam.selector).toBe('""');
      expect(declaredValue(seam.block, "inset"), seam.selector).toBe(
        "0 0 auto",
      );
    }
  });

  it("rules the boundary between a bleeding list and a bleeding table", () => {
    const rules = cssRules(panelCss());
    for (const [first, second] of [
      [".settinglist-bleed", ".table-scroll-bleed"],
      [".table-scroll-bleed", ".settinglist-bleed"],
    ]) {
      const seam = rules.find(
        (rule) => rule.selector === `.panel > ${first} + ${second}`,
      );
      expect(declaredValue(seam?.block ?? "", "border-top")).toBe(
        "1px solid var(--groundSeam)",
      );
    }
  });

  // A group head renders as a `.panel-body`, so the body under it matches the
  // body-after-body seam at equal specificity and only sheet order decides.
  it("draws no seam between a group head and the body it names", () => {
    const rules = cssRules(panelCss());
    const lastContent = (selector: string): number =>
      rules
        .map(
          (rule) =>
            rule.selector === selector &&
            declaredValue(rule.block, "content") !== undefined,
        )
        .lastIndexOf(true);
    const reset = lastContent(".panel-grouphead + .panel-body::before");
    expect(reset).toBeGreaterThanOrEqual(0);
    expect(declaredValue(rules[reset]?.block ?? "", "content")).toBe("none");
    expect(reset).toBeGreaterThan(
      lastContent(".panel-body + .panel-body::before"),
    );
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
    // because a border cannot stop at the card's padding. Asserted on the rule
    // that draws it, so an inset dropped back onto the row's border fails here.
    const line = /(?:^|\n)\.panel-row::before\s*\{([^}]*)\}/.exec(panelCss());
    expect(line).not.toBeNull();
    const drawn = line?.[1] ?? "";
    expect(drawn).toMatch(/background:\s*var\(--groundSeam\)/);
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
    expect(head).toMatch(/border-bottom:\s*1px solid var\(--groundEdge\)/);
    const foot = /(?:^|\n)\.panel-foot\s*\{([^}]*)\}/.exec(css)?.[1] ?? "";
    expect(foot).toMatch(/border-top:\s*1px solid var\(--groundEdge\)/);
    expect(css).not.toMatch(/\.panel-head::after/);
    expect(css).not.toMatch(/\.panel-foot::before/);
  });
});

// The panel's descriptive line owns ONE thing, the interval under it, and that
// is what these read. Spelled in a screen sheet it was corrected in three more,
// so the same sentence stood at three distances from the rows it described.
describe("PanelIntro is the line, and panel.css is its interval", () => {
  it("draws a paragraph and takes the caller's class beside its own", () => {
    render(
      <Panel title="Lead vocabulary">
        <PanelBody>
          <PanelIntro className="listsection-intro">The stages.</PanelIntro>
        </PanelBody>
      </Panel>,
    );
    const line = screen.getByText("The stages.");
    expect(line.tagName).toBe("P");
    expect([...line.classList]).toEqual(["panel-intro", "listsection-intro"]);
  });

  it("spaces the line, drops that space last, and pays a stack the difference", () => {
    const css = panelCss();
    const ruleFor = (selector: string) =>
      cssRules(css).find((candidate) => candidate.selector === selector);
    const rule = (selector: string) => ruleFor(selector)?.block;

    expect(declaredValue(rule(".panel-intro") ?? "", "margin")).toBe(
      "0 0 var(--space-4)",
    );
    // Nothing else: the line reads at the body's type and in the body's ink,
    // and a sheet that gave it either would be a second author for prose the
    // document already decides.
    expect(ruleFor(".panel-intro")?.properties).toEqual(["margin"]);

    // Last in its body the line owes nothing — what follows brings its own top
    // padding, and a trailing margin paid that gap twice.
    expect(
      declaredValue(rule(".panel-intro:last-child") ?? "", "margin-bottom"),
    ).toBe("0");

    // A margin inside a gapped stack ADDS to the gap rather than collapsing
    // into it, so the stack pays the difference and lands on the same interval.
    // Both tokens are read off the stack's own rule rather than named here: a
    // retuned `.form-stack` gap that left this arithmetic behind would put the
    // two spellings back on two rhythms and still pass a literal expectation.
    const gap = declaredValue(
      cssRules(readFileSync(join(here, "atoms.css"), "utf8")).find(
        (candidate) => candidate.selector === ".form-stack",
      )?.block ?? "",
      "gap",
    );
    expect(gap).toBeDefined();
    expect(
      declaredValue(rule(".form-stack > .panel-intro") ?? "", "margin-bottom"),
    ).toBe(`calc(var(--space-4) - ${gap})`);
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

// The coarse-pointer floor every touch target stands at (`.segmented button`,
// `.stat-card-open`); no token carries it.
const TOUCH_TARGET_PX = 44;

// A px value from tokens.css, followed through `var()` to the number.
function tokenPx(name: string): number {
  const value = tokenValue(name);
  const alias = /^var\((--[\w-]+)\)$/.exec(value);
  if (alias) return tokenPx(alias[1]);
  expect(value, `${name} resolves to px`).toMatch(/^\d+px$/);
  return Number.parseFloat(value);
}

// The padding-block one arm of the band declares, read from the arm's own
// `.panel-head` block inside the named query.
function armPadding(query: string): string | undefined {
  const arm = new RegExp(
    `@media \\(${query}\\)\\s*\\{\\s*\\.panel-head\\s*\\{([^}]*)\\}`,
  ).exec(stripComments(panelCss()))?.[1];
  return arm === undefined ? undefined : declaredValue(arm, "padding-block");
}

function spaceToken(value: string | undefined): string {
  const name = /^var\((--space-\d+)\)/.exec(value ?? "")?.[1];
  expect(name, `${value} is a --space-N token`).toBeDefined();
  return name ?? "";
}

describe("the panel head stands at the height every panel shares, and grows only to keep its words", () => {
  const head = () =>
    bandRules(panelCss()).find((rule) => rule.selector === ".panel-head");

  // A floor rather than a height, so a long title wraps and no word is cut.
  it("takes its floor from the house token and wraps what does not fit", () => {
    expect(head()).toBeDefined();
    expect(declaredValue(head()?.block ?? "", "min-height")).toBe(
      "var(--panel-head-h)",
    );
    expect(head()?.properties).not.toContain("height");
    expect(head()?.properties).not.toContain("max-height");
    expect(declaredValue(head()?.block ?? "", "align-items")).toBe("center");
    expect(declaredValue(head()?.block ?? "", "flex-wrap")).toBe("wrap");
    expect(tokenValue("--panel-head-h")).toBe("56px");
  });

  // Wider than a phone, the padding must leave room for a control on a mouse
  // and a 44px target on touch. The 1px rule is inside the floor.
  it("fits a --controlHeight control or a 44px target within the floor, wider than a phone", () => {
    const floor = tokenPx("--panel-head-h");
    const padding = declaredValue(head()?.block ?? "", "padding") ?? "";
    expect(padding).toMatch(/^var\(--space-\d+\) var\(--padPanel\)$/);
    const fine = tokenPx(spaceToken(padding));
    expect(2 * fine + tokenPx("--controlHeight") + 1).toBeLessThanOrEqual(
      floor,
    );
    const coarse = tokenPx(spaceToken(armPadding("pointer: coarse")));
    expect(2 * coarse + TOUCH_TARGET_PX + 1).toBeLessThanOrEqual(floor);
  });

  // One value at every viewport, like the band it is. A touch arm that raised
  // it would give the same panel two heights on one machine.
  it("declares the band's height token once, for every viewport", () => {
    const declarations = tokensCss().match(/--panel-head-h:/g) ?? [];
    expect(declarations).toHaveLength(1);
  });

  // The phone arm comes after the touch arm, so a phone keeps its wider air.
  // That air stands a one-line head with a control above the floor.
  it("gives a phone wider air than the band's own, whatever the pointer", () => {
    const css = stripComments(panelCss());
    expect(css.indexOf("@media (pointer: coarse)")).toBeGreaterThan(-1);
    expect(css.indexOf("@media (max-width: 720px)")).toBeGreaterThan(
      css.indexOf("@media (pointer: coarse)"),
    );
    const phone = tokenPx(spaceToken(armPadding("max-width: 720px")));
    const fine = tokenPx(
      spaceToken(declaredValue(head()?.block ?? "", "padding")),
    );
    expect(phone).toBeGreaterThan(fine);
  });

  // `hyphens` and `hyphenate-limit-chars` inherit, so the limit is the
  // document's, declared once, and every sheet that hyphenates inherits it.
  it("sets the hyphenation limit once, on the document root", () => {
    const root = cssRules(readFileSync(join(here, "base.css"), "utf8")).find(
      (rule) =>
        rule.selector === ":root" &&
        rule.properties.includes("hyphenate-limit-chars"),
    );
    expect(declaredValue(root?.block ?? "", "hyphenate-limit-chars")).toBe(
      "7 3 3",
    );
  });

  // The band grows only by wrapping. A `:has()` rule would grow it for one kind
  // of content, and a page of panels would go ragged.
  it("never sizes the band from what the head carries", () => {
    expect(panelCss()).not.toMatch(/\.panel-head:has\(/);
  });

  // One rule sets the title, so retuning it moves every title. A second rule in
  // this sheet is the tone panels' old habit: each drew its own title and a
  // page then showed the ask and the report in two faces.
  it("sets the title exactly once in the sheet", () => {
    expect(titleTypeRules(panelCss()).map((rule) => rule.selector)).toEqual([
      ".panel-head .panel-title",
    ]);
  });

  // An ellipsis cuts the name a reader finds the zone by. A squeezed badge or
  // button reads as a different control, so nothing else gives way.
  it("wraps the title whole and lets nothing else give way", () => {
    const selector = ".panel-head .panel-title";
    const declared = titleRules(panelCss())
      .map((rule) => rule.block)
      .join(";");
    expect(declaredValue(declared, "white-space"), selector).toBeUndefined();
    expect(declaredValue(declared, "text-overflow"), selector).toBeUndefined();
    expect(declaredValue(declared, "overflow"), selector).toBeUndefined();
    expect(
      declaredValue(declared, "-webkit-line-clamp"),
      selector,
    ).toBeUndefined();
    expect(declaredValue(declared, "hyphens"), selector).toBe("auto");
    expect(declaredValue(declared, "min-width"), selector).toBe("0");
    // The push that keeps an action at the far end rides on the title, so a
    // pair of actions cannot split around it the way a `:last-child` push lets
    // them.
    expect(declaredValue(declared, "margin-right"), selector).toBe("auto");

    const beside = cssRules(panelCss()).find(
      (rule) => rule.selector === ".panel-head > :where(:not(.panel-title))",
    );
    expect(declaredValue(beside?.block ?? "", "flex-shrink")).toBe("0");
    // Capped at the band, so a control that wraps does so inside the head
    // rather than under the pane's clipped edge.
    expect(declaredValue(beside?.block ?? "", "max-width")).toBe("100%");

    // The band itself does not clip: a menu or a tooltip opened from a button
    // in the head has to be able to leave it.
    expect(head()?.properties).not.toContain("overflow");
  });
});

// A tone is a claim — the ordinary ask, the bad news, a machine wrote this —
// and a claim is made in colour. A panel that changed SIZE with its tone would
// read as a different card rather than as the same card in a different mood,
// which is what the ai head did while it hugged its own two lines.
describe("a panel tone tints the head band and never reshapes it", () => {
  // The vocabulary is walked from the component rather than listed again here:
  // a tone spelled twice drifts, and the copy that goes stale is always the one
  // in the test — it passes while the tone nobody added to it ships untinted.
  const TONE_SELECTOR = new RegExp(`^\\.panel-(?:${PANEL_TONES.join("|")})\\b`);
  const toned = () =>
    bandRules(panelCss()).filter((rule) => TONE_SELECTOR.test(rule.selector));

  it("paints every tone the component offers", () => {
    const declared = new Set(
      cssRules(panelCss())
        .map((rule) => /^\.panel-([\w-]+)$/.exec(rule.selector)?.[1])
        .filter((tone): tone is string => tone !== undefined),
    );
    expect([...PANEL_TONES].filter((tone) => !declared.has(tone))).toEqual([]);
  });

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

// ---------------------------------------------------------------------------
// The pane's own inset for a state arm it cannot wrap.
//
// A read whose ready arm is full-bleed `PanelRow`s has nowhere to put a
// `PanelBody`: a body around the read would inset the rows that are supposed
// to reach the pane's edges, so `QueryStates` stands as a direct child of the
// panel and its pending and error arms landed on the pane's ground with no
// inset at all — the loading label printed against the panel's left edge.
//
// So the PANE pays it, once, for every panel in the product. Two screens had
// already spelled that inset in sheets of their own, each under a class only
// that screen knew was a panel, which is what a rule the caller has to
// remember gets you.
// ---------------------------------------------------------------------------

describe("the pane insets the state arm it has no body to wrap", () => {
  it("pays that inset at its own role, on a bare child of the panel", () => {
    const inset = stateArmRoleInsets(panelCss());
    expect([...inset.map((rule) => rule.selector)].sort(stable)).toEqual([
      ".panel > .empty",
      ".panel > .pending",
      '.panel > [role="alert"] > .empty',
    ]);
    // WHICH inset, read off the body rather than restated here: the arm stands
    // in for the rows that will replace it, so a pane retuned to a different
    // token while this rule kept the old one would land the skeleton at one x
    // and the rows it reserved at another.
    const body = /(?:^|\n)\.panel-body\s*\{([^}]*)\}/.exec(panelCss())?.[1];
    const pane = /padding:\s*(var\(--[a-zA-Z0-9-]+\))/.exec(body ?? "")?.[1];
    expect(pane).toBeDefined();
    for (const rule of inset) {
      expect(declaredValue(rule.block, "padding")).toBe(pane);
    }
  });

  it("reads the arm's sentence as a body's: no ground, from the left", () => {
    const rules = cssRules(panelCss());
    const declared = (selector: string, property: string) =>
      rules
        .filter((rule) => rule.selector === selector)
        .map((rule) => declaredValue(rule.block, property))
        .filter((value) => value !== undefined)
        .at(-1);
    for (const arm of [
      ".panel > .empty:not(.empty-plate)",
      '.panel > [role="alert"] > .empty',
    ]) {
      for (const property of ["background", "text-align"]) {
        expect(declared(arm, property)).toBeDefined();
        expect(declared(arm, property)).toBe(
          declared(".panel-body > .empty", property),
        );
      }
    }
  });
});

// An instructional empty state in a panel body is a card of its own, with a
// title, a sentence and a verb. The one-line empty state's flush rule must not
// reach it, or its text sits on its border.
describe("an instructional empty state in a panel body", () => {
  it("keeps its instructional class", () => {
    const { container } = render(
      <PanelBody>
        <EmptyState title="No data purchased">
          A lookup buys details.
        </EmptyState>
      </PanelBody>,
    );
    expect(
      container.querySelector(".panel-body > .empty.empty-instructional"),
    ).not.toBeNull();
  });

  it("is left out of the rule that strips the one-line state's padding", () => {
    const css = readFileSync(
      join(dirname(fileURLToPath(import.meta.url)), "panel.css"),
      "utf8",
    );
    const rules = cssRules(css);
    const paddingOf = (selector: string) =>
      rules
        .filter((rule) => rule.selector === selector)
        .map((rule) => declaredValue(rule.block, "padding"))
        .find((value) => value !== undefined);
    expect(paddingOf(".panel-body > .empty")).toBeUndefined();
    expect(paddingOf(".panel-body > .empty:not(.empty-instructional)")).toBe(
      "0",
    );
  });
});
