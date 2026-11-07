/** @vitest-environment jsdom */
import { readFileSync } from "node:fs";
import { join, relative } from "node:path";
import ts from "typescript";
import { afterEach, describe, expect, it } from "vitest";
import {
  alternativesOf,
  classesOf,
  compounds,
  type Rule,
  readsEveryCombination,
  rulesOf,
  specificity,
  splitTopLevel,
} from "../../scripts/lib/css-rules";
import { mayHoldLiteral } from "../../scripts/lib/literal-text";
import {
  extensionFrontendFiles,
  filesMatching,
  parseSource,
} from "../../scripts/lib/source-tree";
import {
  editedLists,
  ladderSheet,
  OVERLAY_RUNG,
  PAGE_RUNGS,
  PINNED_RUNGS,
} from "../../scripts/lib/surface-ladder";
import {
  ALIAS,
  allRules,
  type Declaration,
  declarations,
  flatFill,
  frontendRoot,
  here,
  IMAGE,
  ladderPath,
  ladderText,
  lists,
  MEMBERS,
  membersOf,
  OPAQUE,
  PSEUDO_ELEMENT,
  probe,
  ref,
  sheets,
  TYPE,
  VAR,
  type Written,
  within,
  written,
} from "../../scripts/lib/surface-sheets";
import { blocks, resolve } from "./tokens-testing";

// A rule outside surfaces.css that names a rung or paints a member overrules the ladder. The ledger holds what predates it; the ceiling keeps it shrinking.
const LEDGER_CEILING = { spellings: 76, states: 6 };

// Joining or leaving the ladder is a decision, so it moves these numbers too.
const LADDER_SIZE = { surfaces: 80, overlays: 19 };

const ledgerPath = join(here, "surfaces.ledger.txt");
const tokensPath = join(here, "tokens.css");
const TOKEN_SHEETS = new Set([tokensPath, join(here, "brand.css")]);

const oneLine = (selector: string) => selector.replace(/\s+/g, " ");

// The selectors tokens.css states its palette under: rules that name no element and hold tokens only.
const THEME_ROOTS = new Set(
  rulesOf([tokensPath])
    .filter(
      (rule) =>
        !rule.selector.includes("*") &&
        compounds(rule.selector).every((part) => classesOf(part).size === 0) &&
        declarations(rule.body).every(({ property }) =>
          property.startsWith("--"),
        ),
    )
    .map((rule) => oneLine(rule.selector)),
);

function definesToken(rule: Rule, property: string): boolean {
  return (
    property.startsWith("--") &&
    TOKEN_SHEETS.has(rule.file) &&
    THEME_ROOTS.has(oneLine(rule.selector))
  );
}

const tokenDefinitions = rulesOf([...TOKEN_SHEETS]).flatMap((rule) =>
  declarations(rule.body).filter(({ property }) =>
    definesToken(rule, property),
  ),
);

// The rungs and every alias of one, either way and in either theme: --paneEdge picks level 1 as surely as --ladder1Edge does. Not a mix: it derives a new colour.
function rungTokens(definitions: readonly Declaration[]): Set<string> {
  const rungs = new Set(
    [...PAGE_RUNGS, OVERLAY_RUNG].flatMap((rung) =>
      [rung.ground, rung.paint, rung.edge].flatMap((value) =>
        [...value.matchAll(VAR)].map((match) => match[1]),
      ),
    ),
  );
  for (let size = 0; size !== rungs.size; ) {
    size = rungs.size;
    for (const { property, value } of definitions) {
      const alias = ALIAS.exec(value)?.[1];
      if (alias === undefined) continue;
      if (rungs.has(alias)) rungs.add(property);
      if (rungs.has(property)) rungs.add(alias);
    }
  }
  return rungs;
}

const RUNGS = rungTokens(tokenDefinitions);
const NAMES_A_RUNG = new RegExp(
  `--(?:${[...RUNGS].map((name) => name.slice(2)).join("|")})(?![\\w-])`,
);

const spelling = (rule: Rule, { property, value }: Declaration) =>
  `${relative(frontendRoot, rule.file)}: ${oneLine(rule.selector)} { ${property}: ${value} }`;

function cssSpellings(rules: readonly Rule[]): string[] {
  return rules.flatMap((rule) => {
    if (rule.file === ladderPath) return [];
    return declarations(rule.body)
      .filter(({ value }) => NAMES_A_RUNG.test(value))
      .filter(({ property }) => !definesToken(rule, property))
      .map((declaration) => spelling(rule, declaration));
  });
}

const SPELLS_A_RUNG = new RegExp(`${NAMES_A_RUNG.source}|var\\(`);

function scriptSpellings(path: string, text: string): string[] {
  if (!mayHoldLiteral(text, SPELLS_A_RUNG)) return [];
  const found: string[] = [];
  const walk = (node: ts.Node) => {
    const part =
      ts.isStringLiteralLike(node) || ts.isTemplateLiteralToken(node)
        ? node.text
        : undefined;
    if (
      part !== undefined &&
      (NAMES_A_RUNG.test(part) ||
        (!ts.isTemplateTail(node) && /var\((?:--)?$/.test(part)))
    ) {
      found.push(`${relative(frontendRoot, path)}: "${part}"`);
    }
    ts.forEachChild(node, walk);
  };
  walk(parseSource(path, text));
  return found;
}

const scripts = [
  ...filesMatching(join(frontendRoot, "src"), /\.tsx?$/),
  ...extensionFrontendFiles(join(frontendRoot, "..", "extensions")),
].filter(
  // Tests plant spellings to probe gates; stories draw tokens as specimens.
  (path) => /\.tsx?$/.test(path) && !/\.(test|stories)\.tsx?$/.test(path),
);

const STATE_LINE = "state: ";

function ledger(): { spellings: string[]; states: string[] } {
  const lines = readFileSync(ledgerPath, "utf8")
    .split("\n")
    .filter((line) => line.trim() !== "" && !line.startsWith("#"));
  return {
    spellings: lines.filter((line) => !line.startsWith(STATE_LINE)),
    states: lines
      .filter((line) => line.startsWith(STATE_LINE))
      .map((line) => line.slice(STATE_LINE.length).split(" | ")[0] ?? ""),
  };
}

// A rule touches a member when its subject may be that element: it names the member's class, or a classless member's element in its context.
function touches(rule: Written, member: Written): boolean {
  if (member.own.size === 0) {
    return rule.type === member.type && within(member.context, rule.context);
  }
  return (
    [...rule.own].some((name) => member.own.has(name)) &&
    (within(rule.own, member.own) || within(member.own, rule.own))
  );
}

const GROUND_FILLS = new Set([
  "var(--ground)",
  "var(--groundHover)",
  "var(--groundStrip)",
  "transparent",
  "none",
]);

function seeThrough(colour: string): boolean {
  return Object.values(blocks).every((palette) => {
    try {
      return !OPAQUE.test(resolve(colour, palette, new Set()));
    } catch {
      return false;
    }
  });
}

// An image layer over a ground is a gradient, or a flat tint the ground shows through.
function keepsTheGround(layer: string): boolean {
  if (!IMAGE.test(layer)) return true;
  const fill = flatFill(layer);
  return fill === undefined || GROUND_FILLS.has(fill) || seeThrough(fill);
}

type Paint = { allowed: boolean; resetsColour: boolean };

function paintOf({ property, value }: Declaration): Paint | undefined {
  if (!/^background(?:-color|-image)?$/.test(property)) return undefined;
  const layers = splitTopLevel(value, ",");
  if (property === "background-color") {
    return { allowed: GROUND_FILLS.has(value), resetsColour: true };
  }
  const images = layers.every(keepsTheGround);
  if (property === "background-image") {
    return { allowed: images, resetsColour: false };
  }
  const colour =
    splitTopLevel(layers.at(-1) ?? "", " ").find(
      (word) =>
        !IMAGE.test(word) &&
        /^(?:var|color-mix|rgba?|hsla?)\(|^#|^transparent$/.test(word),
    ) ?? "transparent";
  return { allowed: images && GROUND_FILLS.has(colour), resetsColour: true };
}

type MemberPaint = Paint & {
  line: string;
  file: string;
  order: number;
  weight: number;
  written: Written;
};

function paintsIn(rules: readonly Rule[]): MemberPaint[] {
  return rules.flatMap((rule, order) => {
    if (rule.file === ladderPath) return [];
    return declarations(rule.body).flatMap((declaration) => {
      const paint = paintOf(declaration);
      if (paint === undefined) return [];
      return alternativesOf(rule.selector)
        .filter(
          (alternative) =>
            !PSEUDO_ELEMENT.test(compounds(alternative).at(-1) ?? ""),
        )
        .map((alternative) => ({
          ...paint,
          line: spelling(rule, declaration),
          file: rule.file,
          order,
          weight: specificity(alternative),
          written: written(alternative),
        }));
    });
  });
}

function covers(
  other: MemberPaint,
  paint: MemberPaint,
  member: Written,
): boolean {
  const outranks =
    other.weight > paint.weight ||
    (other.weight === paint.weight &&
      other.file === paint.file &&
      other.order > paint.order);
  return (
    other.allowed &&
    other.resetsColour &&
    outranks &&
    within(
      other.written.ownSimples,
      paint.written.ownSimples,
      member.ownSimples,
    ) &&
    within(
      other.written.contextSimples,
      paint.written.contextSimples,
      member.contextSimples,
    )
  );
}

function memberPaints(
  rules: readonly Rule[],
  members: readonly Written[],
): string[] {
  const paints = paintsIn(rules);
  const found = members.flatMap((member) => {
    const on = paints.filter((paint) => touches(paint.written, member));
    return on
      .filter(
        (paint) =>
          !paint.allowed && !on.some((other) => covers(other, paint, member)),
      )
      .map((paint) => paint.line);
  });
  return [...new Set(found)].sort();
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
    expect([...THEME_ROOTS].sort()).toEqual([
      ":root",
      ':root:not([data-theme="light"])',
      '[data-theme="dark"]',
    ]);
    expect([...RUNGS].sort()).toEqual(
      expect.arrayContaining([
        "--bgElevated",
        "--pane",
        "--paneEdge",
        "--ladder2",
        "--ladder4Edge",
      ]),
    );
    const pastCap = allRules.filter(
      (rule) => !readsEveryCombination(rule.selector),
    );
    expect(pastCap.length).toBeGreaterThan(0);
    expect(
      pastCap.filter((rule) => rule.file !== ladderPath),
      "a selector past the cap is read one alternative at a time",
    ).toEqual([]);
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
    ].map(([property, value]) => probe(".probe", `${property}: ${value}`));
    expect(cssSpellings(planted)).toHaveLength(planted.length);
    expect(
      scriptSpellings(
        join(here, "probe.tsx"),
        `const a = <div style={{ background: "var(--ladder2)" }} />; const b = \`var(--pane)\`; const c = \`var(\${token})\`; const d = "var(--" + name + ")";`,
      ),
    ).toHaveLength(4);
    const dynamic = 'const d = "var(--" + name;';
    expect(scriptSpellings(join(here, "probe.ts"), dynamic)).toHaveLength(1);
    expect(
      scriptSpellings(join(here, "probe.tsx"), 'const e = "\\u002d-pane";'),
    ).toHaveLength(1);
    expect(
      cssSpellings([
        probe(".probe", `border: 1px solid ${ref("--paneEdgeStrong")}`),
      ]),
    ).toEqual([]);
  });

  it("takes a token's definition only from a theme root of the token sheets", () => {
    const at = (file: string, selector: string): Rule => ({
      ...probe(selector, "--plate: var(--ladder3)"),
      file,
    });
    expect(
      cssSpellings([
        at(tokensPath, '[data-theme="dark"] .x'),
        at(tokensPath, ':root:not([data-theme="light"]) .x'),
        at(tokensPath, ":root .x"),
        at(join(here, "probe.css"), ":root"),
      ]),
    ).toHaveLength(4);
    expect(
      cssSpellings([
        at(tokensPath, ":root"),
        at(tokensPath, '[data-theme="dark"]'),
      ]),
    ).toEqual([]);
  });

  it("closes the rungs over every alias, either way and in either theme", () => {
    expect(
      rungTokens([
        ...tokenDefinitions,
        { property: "--cardGround", value: "var(--ladder2)" },
        { property: "--ladder3", value: ref("--darkStep") },
        { property: "--stepAlias", value: ref("--darkStep") },
      ]),
    ).toEqual(new Set([...RUNGS, "--cardGround", "--darkStep", "--stepAlias"]));
  });

  // Parses every source file, so it takes the tree-walk timeout the other scans use.
  it("names a rung only in surfaces.css, apart from the ledger", {
    timeout: 60_000,
  }, () => {
    const found = [
      ...cssSpellings(allRules),
      ...scripts.flatMap((path) =>
        scriptSpellings(path, readFileSync(path, "utf8")),
      ),
    ].sort();
    const kept = ledger().spellings;
    expect(kept.length).toBeLessThanOrEqual(LEDGER_CEILING.spellings);
    expect(
      found.filter((line) => !kept.includes(line)),
      "paint a pane by joining the ladder (node scripts/surface-ladder.ts <selector>); " +
        "a band or mark that must match its host reads var(--ground) or var(--groundEdge)",
    ).toEqual([]);
    expect(
      kept.filter((line) => !found.includes(line)),
      "remove the ledger lines that no longer match",
    ).toEqual([]);
  });

  it("finds a member painted by a rule that outranks the ladder", () => {
    const members = membersOf([
      ".pane",
      ".host .dialog",
      ".alert.is-quiet",
      ".rows > li",
    ]);
    const flat = (token: string) =>
      `background-image: linear-gradient(var(${token}), var(${token}))`;
    expect(
      memberPaints(
        [
          probe(".pane", "background: var(--dangerBg)"),
          probe(".pane.is-picked", flat("--bgInset")),
          probe(
            ".dialog",
            "background: color-mix(in srgb, red 68%, transparent)",
          ),
          probe(".alert", "background: var(--dangerBg)"),
          probe(".rows > li.is-on", "background-color: var(--accentLight)"),
          probe(".pane::before", "background: var(--danger)"),
          probe(".pane:hover", "background: var(--groundHover)"),
          probe(".pane.is-ai", flat("--aiLight")),
          probe(
            ".pane.is-wash",
            "background-image: radial-gradient(red, blue)",
          ),
          probe(".pane.is-flat", "background: none"),
        ],
        members,
      ),
    ).toHaveLength(5);
    expect(
      memberPaints(
        [
          probe(".dialog", "background: var(--danger)"),
          probe(".host .dialog", "background: var(--ground)"),
          probe(".alert", "background: var(--dangerBg)"),
          probe(".alert.is-quiet", "background-color: var(--ground)"),
        ],
        members,
      ),
    ).toEqual([]);
  });

  it("paints a member only with its ground, apart from the states the ledger names", () => {
    const kept = ledger().states;
    expect(kept.length).toBeLessThanOrEqual(LEDGER_CEILING.states);
    const found = memberPaints(allRules, MEMBERS);
    expect(
      found.filter((line) => !kept.includes(line)),
      "a pane takes its ground from the ladder: read var(--ground), or lay a tint " +
        "as background-image over it",
    ).toEqual([]);
    expect(
      kept.filter((line) => !found.includes(line)),
      "remove the ledger lines that no longer match",
    ).toEqual([]);
  });
});

describe("the ladder's lists", () => {
  it("are one list each, copied into every depth", () => {
    expect(lists.surfaces).toHaveLength(LADDER_SIZE.surfaces);
    expect(lists.overlays).toHaveLength(LADDER_SIZE.overlays);
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

describe("the ladder's command", () => {
  const current = { surfaces: [".a", ".b"], overlays: [".m"] };

  it("adds surfaces and, after --, overlays; --remove takes either out", () => {
    expect(editedLists(current, [".c", "--", ".n"])).toEqual({
      surfaces: [".a", ".b", ".c"],
      overlays: [".m", ".n"],
    });
    expect(editedLists(current, ["--remove", ".b", ".m"])).toEqual({
      surfaces: [".a"],
      overlays: [],
    });
  });

  it("refuses a selector on the ladder twice, or a removal not on it", () => {
    for (const args of [[".a"], ["--", ".m"], [".c", "--", ".c"]]) {
      expect(() => editedLists(current, args)).toThrow(/already on the ladder/);
    }
    for (const args of [["--remove", ".z"], ["--remove"]]) {
      expect(() => editedLists(current, args)).toThrow(/not on the ladder/);
    }
  });

  it("refuses an argument that is a flag, not a selector", () => {
    for (const args of [
      ["--help"],
      [".a", "--remove", ".b"],
      [".c", "--", "-x"],
    ]) {
      expect(() => editedLists(current, args)).toThrow(/not a selector/);
    }
  });
});

describe("a pane's ground is its depth", () => {
  afterEach(() => {
    document.head.replaceChildren();
    document.body.replaceChildren();
  });

  function mount(sheet: string, markup: string | readonly Element[]): void {
    const style = document.createElement("style");
    style.textContent = sheet;
    document.head.replaceChildren(style);
    if (typeof markup === "string") document.body.innerHTML = markup;
    else document.body.replaceChildren(...markup);
  }

  function readAll(property = "--ground"): Record<string, string> {
    return Object.fromEntries(
      [...document.querySelectorAll("[id]")].map((element) => [
        element.id,
        getComputedStyle(element).getPropertyValue(property).trim(),
      ]),
    );
  }

  // The scenarios run on a short list: the sheet is ladderSheet's output (asserted above), and the two tests at the end hold every entry.
  const used = [
    ".panel",
    ".card",
    ".record-card",
    ".page-pane",
    '.stat-strip[data-density="plate"]',
  ];
  const sheet = ladderSheet(
    lists.surfaces.filter(
      (selector) => used.includes(selector) || selector.startsWith(".lt"),
    ),
    lists.overlays.filter((selector) => selector === ".modal"),
  );

  function groundsOf(
    markup: string,
    property = "--ground",
  ): Record<string, string> {
    mount(sheet, markup);
    return readAll(property);
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
        <section class="page-pane"><div class="lt" id="box"></div></section>`),
    ).toEqual({ band: one, box: two });
  });

  // Built by hand, not as HTML: the parser closes a <p> around a nested <div>.
  function built(
    selector: string,
    id = "",
  ): { outer: Element; subject: Element } {
    const elements = compounds(selector).map((compound) => {
      const plain = compound.replace(/:[\w-]+\((?:[^()]|\([^()]*\))*\)/g, "");
      const element = document.createElement(TYPE.exec(plain)?.[0] ?? "div");
      element.classList.add(...classesOf(plain));
      for (const [, name = "", value = ""] of plain.matchAll(
        /\[([\w-]+)(?:="([^"]*)")?\]/g,
      )) {
        element.setAttribute(name, value);
      }
      return element;
    });
    const subject = elements.reduce((parent, child) => {
      parent.append(child);
      return child;
    });
    if (id !== "") subject.id = id;
    return { outer: elements[0] ?? subject, subject };
  }

  function inside(selectors: readonly string[], outer: Element): Element {
    return selectors.reduceRight((held, selector) => {
      const host = built(selector);
      host.subject.append(held);
      return host.outer;
    }, outer);
  }

  // jsdom stops matching a long :where() pair past 47 of the ladder's entries (measured), and sooner for longer names, so the full lists run in chunks.
  const CHUNK = 12;
  const chunks = (list: readonly string[]) =>
    Array.from({ length: Math.ceil(list.length / CHUNK) }, (_, at) =>
      list.slice(at * CHUNK, (at + 1) * CHUNK),
    );

  it("counts every surface on the full list as a level", () => {
    for (const chunk of chunks(lists.surfaces)) {
      const own = chunk.filter((selector) => selector !== ".card");
      const placed = own.flatMap((selector, at) => {
        const lifting = built(selector, `l${at}`);
        lifting.subject.append(built(".card", `c${at}`).outer);
        return [
          lifting.outer,
          inside([".card"], built(selector, `h${at}`).outer),
          inside([".card", ".card", ".modal"], built(selector, `o${at}`).outer),
        ];
      });
      mount(ladderSheet([".card", ...own], [".modal"]), placed);
      const grounds = readAll();
      for (const [at, selector] of own.entries()) {
        expect(
          [`l${at}`, `c${at}`, `h${at}`, `o${at}`].map((id) => grounds[id]),
          selector,
        ).toEqual([one, two, two, two]);
      }
    }
  });

  it("counts again from every overlay on the full list", () => {
    for (const chunk of chunks(lists.overlays)) {
      const placed = chunk.map((selector, at) => {
        const overlay = built(selector, `o${at}`);
        overlay.subject.append(built(".card", `c${at}`).outer);
        return inside([".card", ".card"], overlay.outer);
      });
      mount(ladderSheet([".card"], chunk), placed);
      const grounds = readAll();
      for (const [at, selector] of chunk.entries()) {
        expect([grounds[`o${at}`], grounds[`c${at}`]], selector).toEqual([
          OVERLAY_RUNG.ground,
          two,
        ]);
      }
    }
  });
});
