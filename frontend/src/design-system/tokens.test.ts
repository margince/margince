import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// Pins the light-mode token layer to the canonical Ledger-Green values from the
// spec's design/mockups/app.css :root (design-language §2, ADR-0040). A value
// drifting from the design source of truth — or sliding back to Gradion orange
// or Dispact warm-stone — fails the build.

const here = dirname(fileURLToPath(import.meta.url));
const tokensCss = readFileSync(join(here, "tokens.css"), "utf8");

// The same sheet with its comments removed, for every test that reads
// DECLARATIONS. A declaration and a sentence about a declaration are not the
// same thing, and this file explains most of its values in prose that names
// them; left in, that prose parses as declarations of its own. The shape tests
// below keep reading the raw text, so the line number they report in a failure
// is the line a reader can open.
const tokenDecls = tokensCss.replace(/\/\*[\s\S]*?\*\//g, "");

// Values verbatim from the mockups; comparison normalizes case, whitespace and
// a leading zero before a decimal point so formatting is free but values are not.
//
// The SURFACE rungs and the two content borders are the values that no longer
// match the mockup, and deliberately: each carries --accent's hue at 0.08 of
// --accent's chroma, at the lightness the neutral rung already had. Hue is what
// makes one product out of a light theme and a dark theme whose grounds were
// already ink-green; holding lightness is what keeps the tint out of the
// ladder's steps, the contrast ratios below, and anything laid on top of these
// grounds. The mockups are retired on these seven; this table is what pins them
// in their place, and tokens.css carries the ladder and the contrast steps in
// prose. The derivation itself is not re-run here — the assertions that matter
// are the ordering and the contrast pairs further down, which a wrong tint
// fails whether or not the arithmetic is repeated.
// The inks measured ON --bgChip, and the ONE list two gates read: the contrast
// pairs below pair each of these with the chip fill, and the call-site scan at
// the foot of this file fails a rule that paints --bgChip under an ink missing
// here. Kept as one constant because the two halves are one obligation — a list
// of measured inks that the tree has outgrown is a gate reporting PASS over a
// case it never looked at.
const chipInks: readonly string[] = [
  "--textChip",
  "--textPrimary",
  "--textContent",
  "--tealText",
  "--accentText",
  "--aiText",
];

const canonical: Record<string, string> = {
  "--bgPage": "#f1f5f2",
  "--bgSidebar": "#e6eae7",
  "--bgElevated": "#fbfcfb",
  "--bgCard": "#eaedeb",
  "--bgHover": "#edf0ee",
  "--bgSidebarHover": "#dde1de",
  "--accent": "#0B7A53",
  "--accentLight": "rgba(11,122,83,.09)",
  "--accentMed": "rgba(11,122,83,.17)",
  "--textPrimary": "#15201B",
  "--textContent": "#36433D",
  "--textSecondary": "#5f6a64",
  "--textTertiary": "#9AA6A0",
  "--textMuted": "#CBD2CD",
  "--textMeta": "#5E6C65",
  "--textOnAccent": "#fff",
  "--borderSubtle": "#E3EAE6",
  "--borderStrong": "#D1D8D4",
  "--online": "#22c55e",
  "--teal": "#0E7490",
  "--tealLight": "rgba(14,116,144,.1)",
  "--away": "#fbbf24",
  "--dnd": "#ef4444",
  "--bgRail": "#13231D",
  "--ai": "#5B61D6",
  "--aiLight": "rgba(91,97,214,.08)",
  "--aiMed": "rgba(91,97,214,.30)",
  "--aiText": "#3F45B0",
  "--success": "#15803d",
  "--successBg": "rgba(34,197,94,.12)",
  "--warn": "#92400e",
  "--warnBg": "rgba(251,191,36,.16)",
  "--warnBorder": "rgba(251,191,36,.45)",
  "--danger": "#b91c1c",
  "--dangerBg": "rgba(239,68,68,.1)",
  "--r-xs": "4px",
  "--r-sm": "8px",
  "--r-control": "12px",
  "--r-md": "16px",
  "--r-lg": "20px",
  "--r-full": "9999px",
  "--f-display": '"Outfit",system-ui,sans-serif',
  "--f-body": '"Geist",system-ui,sans-serif',
  "--f-mono": '"Geist Mono",ui-monospace,monospace',
};

function normalize(value: string): string {
  return value
    .toLowerCase()
    .replace(/\s+/g, "")
    .replace(/(^|[^0-9])0\./g, "$1.")
    .replace(/(\.[0-9]*?)0+([^0-9]|$)/g, "$1$2");
}

function parseBlock(css: string, selector: string): Record<string, string> {
  // Parentheses and the colon are escaped too, so a selector carrying a
  // functional pseudo-class (`:root:not([data-theme="light"])`) is matched as
  // the literal text it is rather than compiled into a capture group — which
  // matches nothing, and would report the block as missing.
  const match = css.match(
    new RegExp(`${selector.replace(/[[\]"=():]/g, "\\$&")}\\s*\\{([^}]*)\\}`),
  );
  if (!match) {
    throw new Error(`tokens.css has no ${selector} block`);
  }
  const props: Record<string, string> = {};
  for (const [, name, value] of match[1].matchAll(
    /(--[\w-]+)\s*:\s*([^;]+);/g,
  )) {
    props[name] = value.trim();
  }
  return props;
}

// A media query opened INSIDE :root ends the block early, and every token
// declared below it is stranded in that query — present on the devices the
// query matches and simply absent everywhere else. Nothing else in the tree can
// see it: the property is still declared, so check-space-tokens is satisfied;
// the file still balances, so the formatter is; jsdom resolves no custom
// properties, so no unit test is; and the type checker never reads CSS. The
// only symptom is a page whose spacing, type scale and fonts quietly vanish —
// which is how a `@media (pointer: coarse)` block written to raise the control
// height took the whole --space-* scale down with it, and how the sign-in
// screen's wordmark ended up sitting on top of its own heading.
describe("the token block's shape", () => {
  it("closes :root before any media query opens", () => {
    const rootStart = tokensCss.indexOf(":root {");
    expect(rootStart).toBeGreaterThanOrEqual(0);
    let depth = 0;
    for (let i = rootStart; i < tokensCss.length; i += 1) {
      const char = tokensCss[i];
      if (char === "{") depth += 1;
      if (char === "}") {
        depth -= 1;
        // The block closed cleanly with no @media seen inside it.
        if (depth === 0) return;
      }
      if (depth > 0 && tokensCss.startsWith("@media", i)) {
        const line = tokensCss.slice(0, i).split("\n").length;
        throw new Error(
          `tokens.css:${line} opens a @media inside :root. Every token declared ` +
            `after it is stranded in that query. Put the override AFTER the ` +
            `:root block closes.`,
        );
      }
    }
    throw new Error("tokens.css: the :root block never closes");
  });

  // The scale the whole product measures itself in has to be in the block every
  // document gets, not in one a device may not match.
  it("declares the layout and type scales unconditionally", () => {
    const light = parseBlock(tokenDecls, ":root");
    for (const name of [
      "--space-1",
      "--space-6",
      "--space-16",
      "--control-h",
      "--control-h-sm",
      "--fs-body",
      "--f-body",
      "--phoneNavClearance",
    ]) {
      expect(light[name], `${name} missing from :root`).toBeTruthy();
    }
  });
});

describe("Ledger-Green token layer (B-EP09.1)", () => {
  const light = parseBlock(tokenDecls, ":root");

  it("exports every canonical §2 token with the exact mockup value", () => {
    for (const [name, want] of Object.entries(canonical)) {
      expect(light[name], `${name} missing from :root`).toBeDefined();
      expect(normalize(light[name]), name).toBe(normalize(want));
    }
  });

  it("is Ledger Green — not Gradion orange, not Dispact warm-stone", () => {
    expect(normalize(light["--accent"])).toBe("#0b7a53");
    expect(normalize(light["--bgRail"])).toBe("#13231d");
    const all = normalize(Object.values(light).join(" "));
    expect(all).not.toContain("#ff6b00"); // Gradion orange
  });

  it("keeps brand emerald and success grass-green tonally distinct (§2)", () => {
    expect(normalize(light["--accent"])).not.toBe(normalize(light["--online"]));
  });

  // The surface ladder is a set of RELATIONS, not five independent colours, and
  // every one of them is load-bearing: the rail recedes below the page, a card
  // rises above it, a rail row's hover moves AWAY from the plate its active
  // sibling wears. A retune that keeps all five values plausible and inverts one
  // pair breaks a state the eye reads without breaking anything a value test can
  // see — hover and active becoming the same gesture, or chrome climbing in
  // front of the content it frames. So the ordering is asserted rather than the
  // values, in BOTH themes, from the sheet itself.
  //
  // Dark is not a mirror of light and must not be asserted as one: on a dark
  // ground every surface lifts toward the light, so the ladder runs the other
  // way and only the DIRECTION of each step is shared. Each theme therefore
  // states its own expected order, and both are checked the same way.
  describe("the surface ladder holds its order", () => {
    // Relative luminance, WCAG 2.x §relativeluminancedef. Hex only, which is
    // what every rung on this ladder is — an alpha colour has no luminance of
    // its own, and none of these is one.
    function luminance(hex: string): number {
      const h = hex.trim().replace("#", "");
      const full =
        h.length === 3
          ? h
              .split("")
              .map((d) => d + d)
              .join("")
          : h;
      expect(full, `${hex} is not a 3- or 6-digit hex`).toHaveLength(6);
      const [r, g, b] = [0, 2, 4].map((i) => {
        const c = Number.parseInt(full.slice(i, i + 2), 16) / 255;
        return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
      });
      return 0.2126 * r + 0.7152 * g + 0.0722 * b;
    }

    // A token's channels, whether it is written as a hex or as an rgba(). The
    // alpha comes back too, because a tint's alpha is the whole reason the
    // composite tests below exist.
    function channels(value: string): [number, number, number, number] {
      const v = value.trim();
      if (v.startsWith("#")) {
        const h = v.slice(1);
        const full =
          h.length === 3
            ? h
                .split("")
                .map((d) => d + d)
                .join("")
            : h.slice(0, 6);
        const [r, g, b] = [0, 2, 4].map((i) =>
          Number.parseInt(full.slice(i, i + 2), 16),
        );
        return [r, g, b, 1];
      }
      const inside = v.match(/rgba?\(([^)]+)\)/);
      if (!inside) {
        throw new Error(`${value} is neither a hex nor an rgb()`);
      }
      const parts = inside[1]
        .replace(/\//g, ",")
        .split(",")
        .map((n) => Number.parseFloat(n.trim()));
      return [parts[0], parts[1], parts[2], parts[3] ?? 1];
    }

    // Source-over, the compositing every alpha tint in this file goes through
    // when a browser paints it on a ground.
    function composite(fg: string, bg: string): string {
      const [r, g, b, a] = channels(fg);
      const [br, bg_, bb] = channels(bg);
      const mix = [
        r * a + br * (1 - a),
        g * a + bg_ * (1 - a),
        b * a + bb * (1 - a),
      ].map((n) => Math.round(n));
      return `rgb(${mix.join(", ")})`;
    }

    function contrastOf(fg: string, bg: string): number {
      // Text is opaque in every role measured here; a ground never is not.
      const lf = luminanceOf(fg);
      const lb = luminanceOf(bg);
      const [hi, lo] = lf > lb ? [lf, lb] : [lb, lf];
      return (hi + 0.05) / (lo + 0.05);
    }

    function luminanceOf(value: string): number {
      const [r, g, b] = channels(value);
      const [lr, lg, lb] = [r, g, b].map((n) => {
        const c = n / 255;
        return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
      });
      return 0.2126 * lr + 0.7152 * lg + 0.0722 * lb;
    }

    // Darkest first, as measured. The two themes are deliberately DIFFERENT
    // sequences: light recesses the rail's hover below its ground while dark
    // lifts it above, because a dark surface has only one direction to move in.
    // What both share is the invariant the rail's states depend on — hover on
    // one side of the rail's ground and the active plate (--bgElevated) on the
    // other, so the two states never converge.
    const ladders = {
      light: [
        "--bgSidebarHover",
        "--bgSidebar",
        "--bgCard",
        "--bgHover",
        "--bgPage",
        "--bgElevated",
      ],
      dark: [
        "--bgSidebar",
        "--bgSidebarHover",
        "--bgPage",
        "--bgElevated",
        "--bgCard",
        "--bgHover",
      ],
    };

    for (const [theme, rungs] of Object.entries(ladders)) {
      it(`${theme}: each rung is strictly lighter than the one below it`, () => {
        const block =
          theme === "light"
            ? light
            : { ...light, ...parseBlock(tokenDecls, '[data-theme="dark"]') };
        const measured = rungs.map((name) => {
          expect(block[name], `${name} missing`).toBeDefined();
          return { name, l: luminance(block[name]) };
        });
        for (let i = 1; i < measured.length; i += 1) {
          const below = measured[i - 1];
          const above = measured[i];
          expect(
            above.l,
            `${above.name} (${block[above.name]}) must be lighter than ` +
              `${below.name} (${block[below.name]}) — the ladder inverted`,
          ).toBeGreaterThan(below.l);
        }
      });
    }

    // The lesson that produced this whole block, as a gate. Darkening the
    // grounds by a few percent dropped SEVENTEEN text/ground pairs under
    // 4.5:1 — and the tokens themselves all still looked reasonable in
    // isolation, because a contrast failure is never in a value, it is in a
    // pair. axe on the e2e routes caught two of the seventeen, which is what a
    // route sample can do: it sees the combinations those pages happened to
    // render. This derives the obligation from the palette instead, so the next
    // person who retunes a ground finds out here rather than from a user.
    //
    // Only TEXT roles, and only against grounds they can actually sit on.
    // --textTertiary is deliberately not in the list: it is a decorative tone
    // that never carries prose, and holding it to 4.5:1 would make it
    // --textSecondary.
    it("every text role clears AA on every ground it sits on, both themes", () => {
      const dark = {
        ...light,
        ...parseBlock(tokenDecls, '[data-theme="dark"]'),
      };
      const prose = [
        "--textPrimary",
        "--textContent",
        "--textSecondary",
        "--textMeta",
        "--accentText",
        "--tealText",
      ];
      // Per ground, the roles that can actually be read on it — not a cross
      // product. The two rungs that carry less than everything are the reason
      // this is a map: a hovered RAIL row sets its own ink to --textPrimary
      // (app/shell.css), so the mid-tones never land on --bgSidebarHover, and
      // asserting they do would force that rung lighter than the rail it has to
      // stay darker than. The rail's static ground does carry mid-tone prose —
      // group headings, the entitlement row — so it is held to everything.
      const carries: Record<string, string[]> = {
        "--bgPage": prose,
        "--bgElevated": prose,
        "--bgCard": prose,
        "--bgHover": prose,
        "--bgSidebar": prose,
        "--bgSidebarHover": ["--textPrimary", "--textContent", "--accentText"],
      };
      const failures: string[] = [];
      for (const [theme, pal] of [
        ["light", light],
        ["dark", dark],
      ] as const) {
        for (const [ground, roles] of Object.entries(carries)) {
          for (const role of roles) {
            const ratio = contrastOf(pal[role], pal[ground]);
            if (ratio < 4.5) {
              failures.push(
                `${theme}: ${role} (${pal[role]}) on ${ground} ` +
                  `(${pal[ground]}) = ${ratio.toFixed(2)}:1, needs 4.5:1`,
              );
            }
          }
        }
      }
      expect(failures.join("\n")).toBe("");
    });

    // An alpha tint has no ground of its own: --accentLight over the page and
    // the same tint over a card are two different colours behind the same text,
    // and the second one is always the worse. So each tinted pair is measured
    // COMPOSITED, over every ground the tint can be painted on.
    //
    // The rail is exempt at 4.5 and held to 3:1 instead: the only things wearing
    // an accent tint on the rail are a 26px figure and a glyph, which is where
    // 1.4.3's large-text allowance and 1.4.11's non-text floor apply.
    it("tinted chips clear AA over every ground they composite on", () => {
      const dark = {
        ...light,
        ...parseBlock(tokenDecls, '[data-theme="dark"]'),
      };
      // The three families with a Text token, plus the three STATUS families,
      // which were absent from this list for the reason that is worth stating:
      // the list was the families that HAD a *Text token, so the ones whose ink
      // is the family colour itself were never measured. That is a corpus short
      // of its subject — --success on --successBg is 4.48:1 and shipped, in two
      // writers, until axe caught one of them on one route. Warn and danger
      // clear AA on their own tints today and are here so a retune cannot
      // quietly take that away.
      const pairs = [
        ["--accentText", "--accentLight"],
        ["--tealText", "--tealLight"],
        ["--aiText", "--aiLight"],
        ["--successText", "--successBg"],
        ["--warn", "--warnBg"],
        ["--dangerText", "--dangerBg"],
        // --bgChip is the NEUTRAL member of that list, and the only one whose
        // ink its own family does not fix: it is the fill under a badge, a
        // key-cap, a segmented strip, so it carries whatever the chip's rule
        // sets, and every role that lands on one is measured over it.
        // --textChip is why that list needed a token of its own — a neutral
        // darkening costs what a hued tint does not, and --textMeta had four
        // per cent of headroom to pay with.
        ...chipInks.map((ink) => [ink, "--bgChip"]),
      ] as const;
      const grounds = ["--bgPage", "--bgElevated", "--bgCard", "--bgHover"];
      const failures: string[] = [];
      for (const [theme, pal] of [
        ["light", light],
        ["dark", dark],
      ] as const) {
        for (const [role, tint] of pairs) {
          for (const ground of grounds) {
            const behind = composite(pal[tint], pal[ground]);
            const ratio = contrastOf(pal[role], behind);
            if (ratio < 4.5) {
              failures.push(
                `${theme}: ${role} (${pal[role]}) on ${tint} over ${ground} ` +
                  `= ${behind} = ${ratio.toFixed(2)}:1, needs 4.5:1`,
              );
            }
          }
          const onRail = composite(pal[tint], pal["--bgSidebar"]);
          const railRatio = contrastOf(pal[role], onRail);
          if (railRatio < 3) {
            failures.push(
              `${theme}: ${role} on ${tint} over --bgSidebar = ` +
                `${railRatio.toFixed(2)}:1, needs 3:1 (large text / glyph)`,
            );
          }
        }
      }
      expect(failures.join("\n")).toBe("");
    });

    // The rail's hover and its active plate are the pair a reader actually
    // decodes, so the step between them is asserted as a MAGNITUDE and not only
    // as an order: two rungs one hair apart pass an ordering test and look
    // identical on a screen. 1.1:1 is the floor the ladder's own prose claims.
    for (const theme of ["light", "dark"] as const) {
      it(`${theme}: hover and the active plate are visibly apart`, () => {
        const block =
          theme === "light"
            ? light
            : { ...light, ...parseBlock(tokenDecls, '[data-theme="dark"]') };
        const contrast = (a: string, b: string) => {
          const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
          return (hi + 0.05) / (lo + 0.05);
        };
        expect(
          contrast(block["--bgSidebarHover"], block["--bgElevated"]),
        ).toBeGreaterThan(1.1);
        expect(
          contrast(block["--bgSidebarHover"], block["--bgSidebar"]),
        ).toBeGreaterThan(1.05);
      });
    }
  });

  // The material overlays are the ONLY non-canon literals in this file, and they
  // are here because check-ds-purity.sh excludes tokens.css and nothing else.
  // Pinned so a later "tidy-up" cannot quietly turn them into brand colours.
  it("ships the two material overlays as pure white and pure black", () => {
    expect(normalize(light["--overlayLight"])).toBe("#ffffff");
    expect(normalize(light["--overlayDark"])).toBe("#000000");
  });

  describe("dark palette (data-theme toggle)", () => {
    const dark = parseBlock(tokenDecls, '[data-theme="dark"]');

    it("lightens the accent toward #16A34A (ADR-0040)", () => {
      expect(normalize(dark["--accent"])).toBe("#16a34a");
    });

    it("overrides only tokens the light theme defines — no orphan knobs", () => {
      for (const name of Object.keys(dark)) {
        expect(light[name], `${name} exists only in dark`).toBeDefined();
      }
    });

    it("pins the dark grounds the ladder is measured from", () => {
      const dark = parseBlock(tokenDecls, '[data-theme="dark"]');
      expect(normalize(dark["--bgPage"])).toBe("#0c1311");
      expect(normalize(dark["--bgSidebar"])).toBe("#030504");
      expect(normalize(dark["--bgSidebarHover"])).toBe("#0a100e");
    });

    it("keeps the rail on the shared ink-green field (§2b: the rail is not themed)", () => {
      expect(dark["--bgRail"]).toBeUndefined();
    });

    // A document nobody stamped — an MCP App view whose host stated no theme —
    // gets its dark palette from the platform-preference arm instead, and two
    // copies of one palette are only one palette for as long as somebody
    // checks. Drift here is invisible in both themes of the SPA, which never
    // reads that arm, and shows up only on the surface nobody has open.
    it("answers a dark platform preference with the same palette as the toggle", () => {
      const preferred = parseBlock(
        tokenDecls,
        ':root:not([data-theme="light"])',
      );
      expect(preferred).toEqual(dark);
    });

    // The guard is the half that has to keep the SPA exactly as it renders
    // today: a reader who chose light must stay light on a dark operating
    // system, and only this exclusion makes the media arm lose to that choice.
    it("excludes an explicit light choice from the platform arm", () => {
      const arm = tokenDecls.slice(
        tokenDecls.indexOf("@media (prefers-color-scheme: dark)"),
      );
      expect(arm).toContain(':root:not([data-theme="light"])');
    });
  });
});

// brand.css is the derived layer, and "derived" is the whole guarantee: a literal
// there would be a brand colour the spec has never seen, following neither the
// dark-theme accent lift nor any future palette change. Comments are stripped
// first — the file's own header quotes the accent lift's two hex values, and
// documenting the canon is not inventing a colour.
describe("the derived brand layer", () => {
  const brandCss = readFileSync(join(here, "brand.css"), "utf8").replace(
    /\/\*[\s\S]*?\*\//g,
    "",
  );

  it("invents no colour — every value derives from a canonical token", () => {
    const literal = /#[0-9a-fA-F]{3,8}\b|\b(?:rgba?|hsla?|oklch)\(/;
    for (const [index, line] of brandCss.split("\n").entries()) {
      expect(
        literal.test(line),
        `brand.css line ${index + 1} carries a colour literal — derive it from a token`,
      ).toBe(false);
    }
  });

  it("declares every derived token as a color-mix of a token", () => {
    const declarations = [...brandCss.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)];
    expect(declarations.length).toBeGreaterThan(0);
    for (const [, name, value] of declarations) {
      expect(value, `${name} is not derived`).toMatch(/color-mix\(/);
      expect(value, `${name} mixes no token`).toMatch(/var\(--/);
    }
  });
});

// The pairs above are a LIST of what the tree does, and a list of what a tree
// does is a second copy of it. This derives the corpus instead: every rule under
// src/ that paints --bgChip, plus every rule that draws INSIDE one of those —
// the same element in another state, or a descendant of it — and the ink each
// sets. An unmeasured ink is the failure that matters and the one nothing else
// would see: --textMeta on the chip fill reads 3.99:1 over --bgCard, which is
// the defect --textChip exists to prevent and looks like a perfectly ordinary
// declaration.
//
// The subtree half is not a refinement, it is where the defect actually lived.
// `.segmented` paints the track and sets no ink at all; `.segmented button`
// sets --textMeta a rule later and renders ON that track, and a scan that read
// only the painting rule reported PASS while axe failed two routes. So a rule
// is in scope when ANY compound of its selector carries every class the chip's
// SUBJECT does — its subject, because `.palette-row .type` paints the chip and
// not the row, and reading its first compound instead swept in every sibling
// the row has. That catches `.segmented button`, `.segmented button >
// .segmented-count`, `.badge:hover` and `button.evidence-chip:hover`, and
// correctly leaves `.segmented-mark` alone, since a class name is a token and
// not a prefix. A rule that paints a ground of its OWN is skipped:
// `[aria-pressed="true"]` stands on --bgElevated and its ink answers to that.
//
// What it still cannot see, stated rather than reasoned away: an ink inherited
// from OUTSIDE the chip's subtree. A chip that declares no colour at all reads
// whatever the row around it is drawn in — `.pn-relay-owner`'s own sentence is
// one, its due date having been given an explicit ink and its lead line not —
// and no amount of reading these sheets says what that ground's ink is. The axe
// sweep over the real routes in `e2e/ac.spec.ts` is what covers that shape, and
// it is the gate that found this one.
describe("the chip fill's call sites", () => {
  function stylesheets(dir: string): string[] {
    return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
      const path = join(dir, entry.name);
      if (entry.isDirectory()) {
        return entry.name === "node_modules" || entry.name === "dist"
          ? []
          : stylesheets(path);
      }
      return path.endsWith(".css") ? [path] : [];
    });
  }

  type Rule = { file: string; selector: string; body: string };

  function rules(): Rule[] {
    const sheets = stylesheets(join(here, ".."));
    expect(sheets.length).toBeGreaterThan(0);
    const all: Rule[] = [];
    for (const file of sheets) {
      const sheet = readFileSync(file, "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
      // Innermost brace pairs, so a rule nested in an @media is found as
      // itself and the query around it never matches as a selector.
      for (const [, selector, body] of sheet.matchAll(
        /([^{}]*)\{([^{}]*)\}/g,
      )) {
        for (const one of selector.split(",")) {
          const trimmed = one.trim();
          if (trimmed) all.push({ file, selector: trimmed, body });
        }
      }
    }
    return all;
  }

  // A functional pseudo-class names something OTHER than the element it is
  // written on, so its argument is not part of that element's classes:
  // `:not(.btn)` would otherwise make `btn` a class the chip carries, and the
  // subtree search would then find nothing at all.
  function classesOf(compound: string): Set<string> {
    const bare = compound.replace(/:[\w-]+\([^)]*\)/g, "");
    return new Set([...bare.matchAll(/\.([\w-]+)/g)].map(([, name]) => name));
  }

  function compounds(selector: string): string[] {
    return selector.split(/[\s>+~]+/).filter(Boolean);
  }

  // The SUBJECT of a selector: the compound the rule actually paints, which is
  // the last one. `.palette-row .type` styles the chip, not the row — reading
  // its first compound instead put every `.palette-row` descendant inside a
  // chip it is only a sibling of.
  function subjectClasses(selector: string): Set<string> {
    const parts = compounds(selector);
    return classesOf(parts[parts.length - 1] ?? "");
  }

  // WCAG 1.4.3 exempts an inactive control, which is the whole point of the
  // dimmed tone a disabled segment takes; --textTertiary is out of the contrast
  // corpus above for the same reason and in the same words.
  function isDisabledState(selector: string): boolean {
    return /:disabled|\[disabled\]|\[aria-disabled="true"\]/.test(selector);
  }

  function paintsOwnGround(body: string): boolean {
    return /background(?:-color)?:(?![^;]*var\(--bgChip\))[^;]*(?:var\(|#|rgb)/.test(
      body,
    );
  }

  function inks(body: string): string[] {
    // `(?:^|[;{\s])` is what keeps this off --*-color: the character before a
    // longhand's `color:` is always a hyphen.
    return [...body.matchAll(/(?:^|[;{\s])color:\s*var\((--[\w-]+)\)/g)].map(
      ([, ink]) => ink,
    );
  }

  it("draws on --bgChip only in inks the contrast gate measures", () => {
    const all = rules();
    const chips = all.filter(({ body }) =>
      /background(?:-color)?:[^;]*var\(--bgChip\)/.test(body),
    );
    // A scan that matched nothing would report PASS on an empty corpus.
    expect(chips.length).toBeGreaterThan(0);

    const offenders: string[] = [];
    for (const chip of chips) {
      const wanted = subjectClasses(chip.selector);
      // A chip whose subject carries no class of its own — `.segmented
      // button:active` — is reached through the rule that names the track, so
      // there is nothing here to search on and nothing lost by not searching.
      if (wanted.size === 0) continue;
      const subtree = all.filter((rule) => {
        if (rule === chip) return false;
        return compounds(rule.selector).some((part) => {
          const carried = classesOf(part);
          return [...wanted].every((name) => carried.has(name));
        });
      });
      for (const rule of [chip, ...subtree]) {
        if (isDisabledState(rule.selector)) continue;
        if (rule !== chip && paintsOwnGround(rule.body)) continue;
        for (const ink of inks(rule.body)) {
          if (chipInks.includes(ink)) continue;
          offenders.push(
            `${relative(join(here, ".."), rule.file)}: ${rule.selector} ` +
              `draws on ${chip.selector}'s --bgChip in ${ink}, ` +
              `which no pair measures`,
          );
        }
      }
    }
    expect([...new Set(offenders)].join("\n")).toBe("");
  });

  // The other way the fill goes wrong, and the one that broke the segmented
  // strip: --bgChip painted on a DESCENDANT of something already painted in it.
  // The two composite, the ground goes a step past where the ink was measured,
  // and every pair above is measuring the wrong colour — --textChip on a
  // doubled fill is 3.99:1 in light and 3.45:1 in dark. A chip is one step off
  // its host by construction, so a chip on a chip is never what was meant; the
  // state that wants to look pressed takes a ground from the ladder instead.
  it("never paints --bgChip inside something already painted in it", () => {
    const all = rules();
    const chips = all.filter(({ body }) =>
      /background(?:-color)?:[^;]*var\(--bgChip\)/.test(body),
    );
    expect(chips.length).toBeGreaterThan(0);
    const offenders: string[] = [];
    for (const chip of chips) {
      const wanted = subjectClasses(chip.selector);
      if (wanted.size === 0) continue;
      for (const rule of chips) {
        // A longer chain is a DESCENDANT; the same length carrying the same
        // classes is the same element in another state, which REPLACES the
        // fill rather than stacking on it.
        if (
          compounds(rule.selector).length <= compounds(chip.selector).length
        ) {
          continue;
        }
        const nested = compounds(rule.selector).some((part) => {
          const carried = classesOf(part);
          return [...wanted].every((name) => carried.has(name));
        });
        if (!nested) continue;
        offenders.push(
          `${relative(join(here, ".."), rule.file)}: ${rule.selector} ` +
            `paints --bgChip inside ${chip.selector}, which already does`,
        );
      }
    }
    expect([...new Set(offenders)].join("\n")).toBe("");
  });
});
