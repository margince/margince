/** @vitest-environment jsdom */
import { afterEach, describe, expect, it } from "vitest";
import { classesOf, compounds } from "../../scripts/lib/css-rules";
import {
  editedLists,
  ladderSheet,
  OVERLAY_RUNG,
  PAGE_RUNGS,
  PINNED_RUNGS,
} from "../../scripts/lib/surface-ladder";
import { ladderText, lists, TYPE } from "../../scripts/lib/surface-sheets";

// Joining or leaving the ladder is a decision, so it moves these numbers too.
const LADDER_SIZE = { surfaces: 79, overlays: 19 };

describe("the ladder's lists", () => {
  it("are one list each, copied into every depth", () => {
    expect(lists.surfaces).toHaveLength(LADDER_SIZE.surfaces);
    expect(lists.overlays).toHaveLength(LADDER_SIZE.overlays);
    expect(ladderText).toBe(ladderSheet(lists.surfaces, lists.overlays));
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
