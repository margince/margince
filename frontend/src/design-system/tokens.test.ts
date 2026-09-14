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
  // The status family, one hue per tone. The base token is the hue itself; the
  // Text token is the same hue at the ink share its pane needs; the tints
  // derive from the base. The hue is what is pinned here — a share moving is a
  // contrast decision the sweeps below judge, but the HUE moving is a new
  // colour in the palette.
  "--success":
    "color-mix(in oklab, lab(71.4376% -59.4106 38.0321), var(--textPrimary) 8%)",
  "--successText":
    "color-mix(in oklab, lab(71.4376% -59.4106 38.0321), var(--textPrimary) 49%)",
  "--successBg": "color-mix(in srgb, var(--success) 12%, transparent)",
  "--warn":
    "color-mix(in oklab, lab(74.4448% 23.7172 71.6451), var(--textPrimary) 8%)",
  "--warnText":
    "color-mix(in oklab, lab(74.4448% 23.7172 71.6451), var(--textPrimary) 52%)",
  "--warnBg": "color-mix(in srgb, var(--warn) 16%, transparent)",
  "--warnBorder": "color-mix(in srgb, var(--warn) 45%, transparent)",
  "--danger":
    "color-mix(in oklab, lab(57.4234% 73.5589 48.0136), var(--textPrimary) 8%)",
  "--dangerText":
    "color-mix(in oklab, lab(57.4234% 73.5589 48.0136), var(--textPrimary) 34%)",
  "--dangerBg": "color-mix(in srgb, var(--danger) 10%, transparent)",
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
      const [lr, lg, lb] = linearOf(value);
      return 0.2126 * lr + 0.7152 * lg + 0.0722 * lb;
    }

    // Every value the sweeps below measure, as a hex or an rgba(). Most of the
    // palette already is one; the status family is not — --success is a
    // color-mix() of a lab() hue and the theme's own ink, and --successBg is a
    // mix of that. A sweep that cannot read those forms stops measuring the
    // family it was written for and still reports PASS, which is the one way a
    // gate must not break. So the declared value is resolved here, following
    // exactly the forms tokens.css writes and THROWING on anything else.
    function topLevelArgs(inside: string): string[] {
      const args: string[] = [];
      let depth = 0;
      let start = 0;
      for (let i = 0; i < inside.length; i += 1) {
        if (inside[i] === "(") depth += 1;
        if (inside[i] === ")") depth -= 1;
        if (inside[i] === "," && depth === 0) {
          args.push(inside.slice(start, i).trim());
          start = i + 1;
        }
      }
      args.push(inside.slice(start).trim());
      return args;
    }

    // CSS lab() is CIELAB on the D50 white point (CSS Color 4 §7), so sRGB is
    // three conversions away and none is optional: Lab to XYZ under D50, the
    // Bradford adaptation to D65, then D65 XYZ to linear sRGB. Skipping the
    // adaptation moves every hue here by more than the headroom being
    // measured.
    function labToLinear(L: number, a: number, b: number): number[] {
      const fy = (L + 16) / 116;
      const fx = fy + a / 500;
      const fz = fy - b / 200;
      const epsilon = 216 / 24389;
      const kappa = 24389 / 27;
      const x = fx ** 3 > epsilon ? fx ** 3 : (116 * fx - 16) / kappa;
      const y = L > kappa * epsilon ? fy ** 3 : L / kappa;
      const z = fz ** 3 > epsilon ? fz ** 3 : (116 * fz - 16) / kappa;
      const d50 = [0.9642956764295677, 1, 0.8251046025104602];
      const [xd, yd, zd] = [x * d50[0], y * d50[1], z * d50[2]];
      const toD65 = [
        [0.955473452704218, -0.02309853687426142, 0.0632593086610217],
        [-0.02836970696320814, 1.0099954580058226, 0.021041398966943],
        [0.0123140016883199, -0.02050769643347791, 1.3303659366080753],
      ];
      const toLinear = [
        [3.2409699419045226, -1.537383177570094, -0.4986107602930034],
        [-0.9692436362808796, 1.8759675015077202, 0.04155505740717559],
        [0.05563007969699366, -0.20397695888897652, 1.0569715142428786],
      ];
      const xyz = toD65.map((row) => row[0] * xd + row[1] * yd + row[2] * zd);
      return toLinear.map((row) =>
        row.reduce((sum, coef, i) => sum + coef * xyz[i], 0),
      );
    }

    // Oklab, the space tokens.css interpolates the status family in (CSS Color
    // 4 §10). A mix there is a straight interpolation of these three
    // coordinates, which is why the tokens are written in it: one ink share
    // means the same thing to the eye at every hue in the family.
    function linearToOklab(rgb: number[]): number[] {
      const l =
        0.4122214708 * rgb[0] + 0.5363325363 * rgb[1] + 0.0514459929 * rgb[2];
      const m =
        0.2119034982 * rgb[0] + 0.6806995451 * rgb[1] + 0.1073969566 * rgb[2];
      const s =
        0.0883024619 * rgb[0] + 0.2817188376 * rgb[1] + 0.6299787005 * rgb[2];
      const [l_, m_, s_] = [l, m, s].map((v) => Math.cbrt(v));
      return [
        0.2104542553 * l_ + 0.793617785 * m_ - 0.0040720468 * s_,
        1.9779984951 * l_ - 2.428592205 * m_ + 0.4505937099 * s_,
        0.0259040371 * l_ + 0.7827717662 * m_ - 0.808675766 * s_,
      ];
    }

    function oklabToLinear(lab: number[]): number[] {
      const l_ = lab[0] + 0.3963377774 * lab[1] + 0.2158037573 * lab[2];
      const m_ = lab[0] - 0.1055613458 * lab[1] - 0.0638541728 * lab[2];
      const s_ = lab[0] - 0.0894841775 * lab[1] - 1.291485548 * lab[2];
      const [l, m, s] = [l_ ** 3, m_ ** 3, s_ ** 3];
      return [
        4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
        -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
        -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
      ];
    }

    // Linear-light channels to the hex a browser would paint, gamut-clipped per
    // channel the way a browser clips an out-of-gamut lab().
    function hexOf(linear: number[]): string {
      const bytes = linear.map((channel) => {
        const clipped = Math.min(Math.max(channel, 0), 1);
        const encoded =
          clipped <= 0.0031308
            ? clipped * 12.92
            : 1.055 * clipped ** (1 / 2.4) - 0.055;
        return Math.round(encoded * 255)
          .toString(16)
          .padStart(2, "0");
      });
      return `#${bytes.join("")}`;
    }

    function linearOf(value: string): number[] {
      const [r, g, b] = channels(value);
      return [r, g, b].map((n) => {
        const c = n / 255;
        return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
      });
    }

    // A colour and the share of the mix it takes, split off one argument. It
    // throws rather than defaulting: a spelling it cannot read would otherwise
    // be measured as the literal text it failed to parse.
    function shareOf(arg: string, whole: string): [string, number] {
      const share = arg.match(/^(.*)\s(\d+(?:\.\d+)?)%$/);
      if (!share) {
        throw new Error(`${whole}: "${arg}" states no percentage`);
      }
      return [share[1], Number.parseFloat(share[2]) / 100];
    }

    function resolve(
      value: string,
      pal: Record<string, string>,
      seen: ReadonlySet<string>,
    ): string {
      const v = value.trim();
      const named = v.match(/^var\((--[\w-]+)\)$/);
      if (named) {
        if (seen.has(named[1])) {
          throw new Error(`${named[1]} resolves to itself`);
        }
        if (!pal[named[1]]) {
          throw new Error(`${value} names undeclared ${named[1]}`);
        }
        return resolve(pal[named[1]], pal, new Set([...seen, named[1]]));
      }
      const lab = v.match(/^lab\(\s*([\d.]+)%\s+(-?[\d.]+)\s+(-?[\d.]+)\s*\)$/);
      if (lab) {
        const [L, a, b] = lab.slice(1, 4).map(Number.parseFloat);
        return hexOf(labToLinear(L, a, b));
      }
      const mix = v.match(/^color-mix\(\s*in (oklab|srgb)\s*,([\s\S]*)\)$/);
      if (!mix) return v;
      const args = topLevelArgs(mix[2]);
      if (args.length !== 2) {
        throw new Error(`${value} is not a two-colour mix`);
      }
      // color-mix(in srgb, C P%, transparent) is how a tint derives from its
      // base. sRGB interpolation is premultiplied and transparent contributes
      // nothing, so the result is C at alpha P/100 — the same colour the rgba()
      // literals used to spell, now unable to drift from the hue it tints.
      if (mix[1] === "srgb") {
        if (args[1] !== "transparent") {
          throw new Error(`${value} mixes something other than transparent`);
        }
        const [colour, alpha] = shareOf(args[0], value);
        const [r, g, b] = channels(resolve(colour, pal, seen));
        return `rgba(${r}, ${g}, ${b}, ${alpha})`;
      }
      const [inkName, part] = shareOf(args[1], value);
      const hue = linearToOklab(linearOf(resolve(args[0], pal, seen)));
      const ink = linearToOklab(linearOf(resolve(inkName, pal, seen)));
      return hexOf(
        oklabToLinear(hue.map((n, i) => n * (1 - part) + ink[i] * part)),
      );
    }

    // One theme's palette with every colour resolved. Non-colour tokens — the
    // space scale, the fonts, a shadow — fall through untouched: they are not
    // one of the forms above and nothing here measures them.
    function resolved(pal: Record<string, string>): Record<string, string> {
      return Object.fromEntries(
        Object.entries(pal).map(([name, value]) => [
          name,
          resolve(value, pal, new Set([name])),
        ]),
      );
    }

    // The two palettes every contrast assertion below reads.
    const themes = {
      light: resolved(light),
      dark: resolved({
        ...light,
        ...parseBlock(tokenDecls, '[data-theme="dark"]'),
      }),
    } as const;

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
    // contact who retunes a ground finds out here rather than from a user.
    //
    // Only TEXT roles, and only against grounds they can actually sit on.
    // --textTertiary is deliberately not in the list: it is a decorative tone
    // that never carries prose, and holding it to 4.5:1 would make it
    // --textMeta.
    it("every text role clears AA on every ground it sits on, both themes", () => {
      // The status Text tokens are in this list and their bases are not, which
      // is the split the family is built on: --successText and its siblings ARE
      // prose roles — a form error, a stale-value caveat, a badge's ink — while
      // --success is a fill, a bar and a 17px figure, measured at the figure's
      // own size and not at the prose floor.
      const prose = [
        "--textPrimary",
        "--textContent",
        "--textMeta",
        "--accentText",
        "--tealText",
        "--successText",
        "--warnText",
        "--dangerText",
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
        // A control FILLED with a status tone is a ground too: its label is
        // read on the fill, and that fill is the tone's TEXT token, never the
        // base hue — the base is tuned to be SEEN at a figure's or a bar's
        // size and nothing clears 4.5:1 on it (white 2.55:1 on --success, the
        // best dark ink 4.11:1 on --danger, which is how a destructive button
        // filled with --danger shipped as an axe failure). Its ink is the one
        // in the palette that FLIPS with the theme, because the ground under
        // it does: a Text red is deep in light and bright in dark.
        "--successText": ["--textOnStatusControl"],
        "--warnText": ["--textOnStatusControl"],
        "--dangerText": ["--textOnStatusControl"],
      };
      const failures: string[] = [];
      for (const [theme, pal] of Object.entries(themes)) {
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
      // Every family with a Text token, which is now every family that tints:
      // --warn used to be measured here as its own ink, because the list was
      // the families that HAD a *Text token and the ones whose ink was the
      // family colour itself went unmeasured — a corpus short of its subject,
      // which is how --success on --successBg shipped at 4.48:1 in two writers
      // until axe caught one of them on one route. Each tone now has the ink
      // its tint needs, and each is paired with the tint it lands on.
      const pairs = [
        ["--accentText", "--accentLight"],
        ["--tealText", "--tealLight"],
        ["--aiText", "--aiLight"],
        ["--successText", "--successBg"],
        ["--warnText", "--warnBg"],
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
      for (const [theme, pal] of Object.entries(themes)) {
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

    // The one place the status family repeats itself: a base token and its Text
    // sibling each name the lab() hue, because the SHARE is what the pair is
    // for and a share needs a colour to take it from. Two writers of one hue,
    // so a gate holds them equal rather than a comment asking the next author
    // to remember.
    it("declares each status Text token on the same hue as its base", () => {
      const hueOf = (value: string) => value.match(/lab\([^)]*\)/)?.[0];
      for (const tone of ["success", "warn", "danger"]) {
        const base = hueOf(light[`--${tone}`]);
        expect(base, `--${tone} names no lab() hue`).toBeDefined();
        expect(hueOf(light[`--${tone}Text`]), `--${tone}Text`).toBe(base);
      }
    });

    // The other half of that claim: the base is stated ONCE. A verdict is the
    // same verdict at any hour, and the ink folded into it follows the theme on
    // its own because --textPrimary does. A dark block that redefined one would
    // be a second answer to a question this family answers by not asking it.
    it("states each status hue once — no dark block redefines a base", () => {
      for (const selector of [
        '[data-theme="dark"]',
        ':root:not([data-theme="light"])',
      ]) {
        const block = parseBlock(tokenDecls, selector);
        for (const tone of ["success", "warn", "danger"]) {
          expect(
            block[`--${tone}`],
            `${selector} redefines --${tone}`,
          ).toBeUndefined();
        }
      }
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
