/** @vitest-environment jsdom */
import { readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { afterEach, describe, expect, it } from "vitest";
import {
  appStylesheets,
  classesOf,
  compounds,
  type Rule,
  rulesOf,
  selectorList,
  splitTopLevel,
} from "../../scripts/lib/css-rules";
import {
  extensionFrontendFiles,
  filesMatching,
  parseSource,
} from "../../scripts/lib/source-tree";
import {
  ladderLists,
  ladderSheet,
  OVERLAY_RUNG,
  PAGE_RUNGS,
  PINNED_RUNGS,
  UNDERLAY_HOSTS,
} from "../../scripts/lib/surface-ladder";
import { parseBlock, themes, tokenDecls } from "./tokens-testing";

// A rule outside surfaces.css that names a rung picks a pane's depth by hand.
// The ledger holds the spellings older than the ladder, and only shrinks.

const here = dirname(fileURLToPath(import.meta.url));
const frontendRoot = join(here, "..", "..");
const ladderPath = join(here, "surfaces.css");
const ladderText = readFileSync(ladderPath, "utf8");
const ledgerPath = join(here, "surfaces.ledger.txt");

const VAR = /var\((--[\w-]+)\)/g;

// The rungs, and the light tokens each stands for: --paneEdge picks level 1 as
// surely as --ladder1Edge does.
function rungTokens(): Set<string> {
  const light = parseBlock(tokenDecls, ":root");
  const named = [...PAGE_RUNGS, OVERLAY_RUNG].flatMap((rung) =>
    [rung.ground, rung.paint, rung.edge].flatMap((value) =>
      [...value.matchAll(VAR)].map((match) => match[1]),
    ),
  );
  const behind = named.flatMap((name) =>
    [...(light[name] ?? "").matchAll(VAR)].map((match) => match[1]),
  );
  return new Set([...named, ...behind]);
}

const RUNGS = rungTokens();
const NAMES_A_RUNG = new RegExp(
  `--(?:${[...RUNGS].map((name) => name.slice(2)).join("|")})(?![\\w-])`,
);

// A custom property declared on a theme root is a token's definition: brand
// derives its washes there, and tokens.css states the rungs themselves.
const THEME_ROOT = /^(?::root|\[data-theme=)/;

function declarations(body: string): { property: string; value: string }[] {
  return splitTopLevel(body, ";").flatMap((part) => {
    const colon = part.indexOf(":");
    if (colon < 0) return [];
    return [
      {
        property: part.slice(0, colon).trim(),
        value: part
          .slice(colon + 1)
          .trim()
          .replace(/\s+/g, " "),
      },
    ];
  });
}

/** Every declaration outside the ladder that names a rung, one line each. */
function cssSpellings(rules: readonly Rule[]): string[] {
  return rules.flatMap((rule) => {
    if (rule.file === ladderPath) return [];
    const path = relative(frontendRoot, rule.file);
    const selector = rule.selector.replace(/\s+/g, " ");
    return declarations(rule.body)
      .filter(({ value }) => NAMES_A_RUNG.test(value))
      .filter(
        ({ property }) =>
          !(property.startsWith("--") && THEME_ROOT.test(selector)),
      )
      .map(
        ({ property, value }) =>
          `${path}: ${selector} { ${property}: ${value} }`,
      );
  });
}

/** Every string a script or component holds that names a rung. */
function scriptSpellings(path: string, text: string): string[] {
  const found: string[] = [];
  const walk = (node: ts.Node) => {
    if (
      (ts.isStringLiteralLike(node) ||
        ts.isTemplateHead(node) ||
        ts.isTemplateMiddle(node) ||
        ts.isTemplateTail(node)) &&
      NAMES_A_RUNG.test(node.text)
    ) {
      found.push(`${relative(frontendRoot, path)}: "${node.text}"`);
    }
    ts.forEachChild(node, walk);
  };
  walk(parseSource(path, text));
  return found;
}

const sheets = appStylesheets(frontendRoot);
const scripts = [
  ...filesMatching(join(frontendRoot, "src"), /\.tsx?$/),
  ...extensionFrontendFiles(join(frontendRoot, "..", "extensions")),
].filter(
  // Tests plant spellings to probe gates; stories draw tokens as specimens.
  (path) => /\.tsx?$/.test(path) && !/\.(test|stories)\.tsx?$/.test(path),
);

function ledger(): string[] {
  return readFileSync(ledgerPath, "utf8")
    .split("\n")
    .filter((line) => line.trim() !== "" && !line.startsWith("#"));
}

describe("the nesting ladder has one painter", () => {
  it("reads the whole tree", () => {
    expect(sheets.length).toBeGreaterThan(250);
    for (const sheet of ["recordview.css", "conversation.css", "view.css"]) {
      expect(
        sheets.some((path) => path.endsWith(sheet)),
        sheet,
      ).toBe(true);
    }
    expect(scripts.length).toBeGreaterThan(500);
    expect([...RUNGS].sort()).toEqual(
      expect.arrayContaining([
        "--bgElevated",
        "--pane",
        "--paneEdge",
        "--ladder2",
        "--ladder4Edge",
      ]),
    );
  });

  it("finds a rung however it is spelled", () => {
    const planted: Rule[] = [
      ["background-image", "linear-gradient(var(--pane), var(--pane))"],
      ["background", 'url("a.svg") no-repeat, var(--ladder2)'],
      ["--plate", "var(--bgElevated)"],
      ["box-shadow", "0 0 0 100vw var(--ladder4)"],
      ["background-color", "var(--ladder3)"],
      ["border", "1px solid var(--paneEdge)"],
      ["border-color", "var(--ladder3Edge)"],
    ].map(([property, value]) => ({
      file: join(here, "probe.css"),
      selector: ".probe",
      body: `${property}: ${value}`,
    }));
    expect(cssSpellings(planted)).toHaveLength(planted.length);
    expect(
      scriptSpellings(
        join(here, "probe.tsx"),
        'const a = <div style={{ background: "var(--ladder2)" }} />; const b = `var(--pane)`;',
      ),
    ).toHaveLength(2);
    expect(
      cssSpellings([
        {
          file: join(here, "probe.css"),
          selector: ".probe",
          body: "border: 1px solid var(--paneEdgeStrong)",
        },
      ]),
    ).toEqual([]);
  });

  it("names a rung only in surfaces.css, apart from the ledger", () => {
    const found = [
      ...cssSpellings(rulesOf(sheets)),
      ...scripts.flatMap((path) =>
        scriptSpellings(path, readFileSync(path, "utf8")),
      ),
    ].sort();
    const kept = ledger();
    expect(
      found.filter((line) => !kept.includes(line)),
      "paint a pane by joining the ladder (node scripts/surface-ladder.ts <selector>); " +
        "a band or mark that must match its host reads var(--ground) or var(--groundEdge)",
    ).toEqual([]);
    expect(
      kept.filter((line) => !found.includes(line)),
      "the ledger only shrinks: remove the lines that no longer match",
    ).toEqual([]);
  });
});

describe("the ladder's lists", () => {
  const lists = ladderLists(ladderText);

  it("are one list each, copied into every depth", () => {
    expect(lists.surfaces.length).toBeGreaterThan(40);
    expect(lists.overlays.length).toBeGreaterThan(10);
    expect(ladderText).toBe(ladderSheet(lists.surfaces, lists.overlays));
  });

  it("name classes some other sheet styles", () => {
    const styled = new Set(
      rulesOf(sheets.filter((path) => path !== ladderPath)).flatMap((rule) =>
        compounds(rule.selector).flatMap((part) => [...classesOf(part)]),
      ),
    );
    const named = [
      ...lists.surfaces,
      ...lists.overlays,
      ...PINNED_RUNGS.map((pin) => pin.selector),
    ].flatMap((selector) =>
      compounds(selector).flatMap((part) => [...classesOf(part)]),
    );
    expect(named.filter((name) => !styled.has(name))).toEqual([]);
  });
});

describe("a pane's ground is its depth", () => {
  afterEach(() => {
    document.head.replaceChildren();
    document.body.replaceChildren();
  });

  // jsdom stops matching a :where() past about fifty selectors; the sheet is
  // ladderSheet's output (asserted above), so a short list runs the same rules.
  const { surfaces, overlays } = ladderLists(ladderText);
  const used = [
    ".panel",
    ".card",
    ".record-card",
    ".record-split-sheet",
    '.stat-strip[data-density="plate"]',
  ];
  const sheet = ladderSheet(
    surfaces.filter(
      (selector) => used.includes(selector) || selector.startsWith(".lt"),
    ),
    overlays.filter((selector) => selector === ".modal"),
  );

  function groundsOf(
    markup: string,
    property = "--ground",
  ): Record<string, string> {
    const style = document.createElement("style");
    style.textContent = sheet;
    document.head.append(style);
    document.body.innerHTML = markup;
    return Object.fromEntries(
      [...document.querySelectorAll("[id]")].map((element) => [
        element.id,
        getComputedStyle(element).getPropertyValue(property).trim(),
      ]),
    );
  }

  const [one, two, three, four] = PAGE_RUNGS.map((rung) => rung.ground);

  it("counts from the page, and stays at the last rung past four", () => {
    expect(
      groundsOf(`
        <section class="panel" id="p1"><div>
          <div class="card" id="p2"><div class="panel" id="p3">
            <div class="record-card" id="p4"><div class="panel" id="p5"></div></div>
          </div></div>
        </div></section>`),
    ).toEqual({ p1: one, p2: two, p3: three, p4: four, p5: four });
  });

  it("hands each level its own edge, and a head inside a pane the pane's", () => {
    const edges = PAGE_RUNGS.map((rung) => rung.edge);
    expect(
      groundsOf(
        `
        <section class="panel" id="p1"><div class="panel-head" id="h1"></div>
          <div class="card" id="p2"><div class="panel" id="p3">
            <div class="panel-head" id="h3"></div>
            <div class="record-card" id="p4"></div>
          </div></div>
        </section>
        <div class="modal" id="o"><div class="panel" id="o2"></div></div>`,
        "--groundEdge",
      ),
    ).toEqual({
      p1: edges[0],
      h1: edges[0],
      p2: edges[1],
      p3: edges[2],
      h3: edges[2],
      p4: edges[3],
      o: OVERLAY_RUNG.edge,
      o2: edges[1],
    });
  });

  it("seams a pane in the house hairline at level 1 and in its edge below", () => {
    const seams = PAGE_RUNGS.map((rung) => rung.seam);
    expect(
      groundsOf(
        `
        <section class="panel" id="p1"><div class="card" id="p2">
          <div class="panel" id="p3"><div class="record-card" id="p4"></div></div>
        </div></section>
        <div class="modal" id="o"></div>`,
        "--groundSeam",
      ),
    ).toEqual({
      p1: seams[0],
      p2: seams[1],
      p3: seams[2],
      p4: seams[3],
      o: OVERLAY_RUNG.seam,
    });
    expect(seams.slice(1)).toEqual(
      PAGE_RUNGS.slice(1).map((rung) => rung.edge),
    );
  });

  it("keeps a pinned part on its rung at any depth", () => {
    const [tools, strip] = PINNED_RUNGS.map((pin) => PAGE_RUNGS[pin.level - 1]);
    const markup = `
      <section class="panel"><div class="lt-tools" id="t1"></div>
        <div class="card"><div class="panel">
          <div class="lt-tools" id="t4"></div>
          <section class="stat-strip" data-density="plate" id="s4"></section>
        </div></div>
      </section>`;
    expect(groundsOf(markup)).toEqual({
      t1: tools.ground,
      t4: tools.ground,
      s4: four,
    });
    expect(groundsOf(markup, "--groundEdge")).toEqual({
      t1: tools.edge,
      t4: tools.edge,
      s4: strip.edge,
    });
  });

  it("counts again from an overlay, wherever the overlay sits", () => {
    expect(
      groundsOf(`
        <section class="panel"><div class="panel"><div class="panel">
          <div class="modal" id="o">
            <div class="panel" id="o2"><div class="card" id="o3">
              <div class="panel" id="o4"></div>
            </div></div>
          </div>
        </div></div></section>`),
    ).toEqual({ o: OVERLAY_RUNG.ground, o2: two, o3: three, o4: four });
  });

  it("leaves a list inside a panel as a band of that panel", () => {
    expect(
      groundsOf(`
        <section class="panel"><div class="lt" id="band"></div></section>
        <section class="record-split-sheet"><div class="lt" id="box"></div></section>`),
    ).toEqual({ band: one, box: two });
  });
});

// A pseudo-element lies under its host's content, an element under its parent's.
function underlayHosts(): string[] {
  const layers = rulesOf(sheets)
    .filter((rule) => rule.file !== ladderPath)
    .filter((rule) =>
      declarations(rule.body).some(
        ({ property, value }) =>
          property === "z-index" && value.startsWith("-"),
      ),
    )
    .flatMap((rule) => selectorList(rule.selector));
  const hosts = layers.map((layer) => {
    const host = layer.replace(/::?(?:before|after)$/, "");
    return host === layer ? `:has(> ${layer})` : host;
  });
  return [...new Set(hosts)].sort();
}

const OPAQUE = /^#[0-9a-f]{6}$/i;

describe("a layer under a pane stays under it", () => {
  it("gives every rung an opaque ground in both themes", () => {
    for (const rung of [...PAGE_RUNGS, OVERLAY_RUNG]) {
      for (const [theme, palette] of Object.entries(themes)) {
        const name = rung.ground.match(/^var\((--[\w-]+)\)$/)?.[1] ?? "";
        expect(palette[name], `${rung.ground} in ${theme}`).toMatch(OPAQUE);
      }
    }
  });

  it("paints the ground of a pane on every host the tree stacks a layer in", () => {
    expect(underlayHosts()).toEqual([...UNDERLAY_HOSTS].sort());
  });

  it("reads an opaque fill for a layer that hides the border-box one under it", () => {
    const knockouts = rulesOf(sheets).flatMap((rule) => {
      const paint = declarations(rule.body)
        .filter(({ property }) => property.startsWith("background"))
        .map(({ value }) => value)
        .join(", ");
      if (!paint.includes("padding-box") || !paint.includes("border-box"))
        return [];
      const fills = [
        ...paint.matchAll(/linear-gradient\(\s*(var\(--[\w-]+\)),\s*\1\s*\)/g),
      ].map((match) => match[1] ?? "");
      return [{ selector: rule.selector, fills }];
    });
    expect(knockouts.length).toBeGreaterThan(0);
    for (const { selector, fills } of knockouts) {
      expect(fills.length, selector).toBeGreaterThan(0);
      for (const fill of fills) {
        if (fill === "var(--ground)") continue;
        const name = fill.slice(4, -1);
        for (const [theme, palette] of Object.entries(themes)) {
          expect(palette[name], `${selector}: ${fill} in ${theme}`).toMatch(
            OPAQUE,
          );
        }
      }
    }
  });
});
