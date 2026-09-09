// Edge-padding gate: no text printed against the edge of the box that holds it.
//
// The defect this catches is invisible to a stylesheet reader, because it is
// never written in one rule. `.card` pays `--padCard`; a class the caller hands
// the same element re-declares `padding` and leaves the INLINE half at `0`; the
// later rule wins, and every bare empty state in the product printed its
// sentence hard against the left edge of the plate holding it. Nothing that
// reads CSS can see that — the two rules are correct apart and wrong together,
// and which wins depends on the order the bundler emitted them in.
//
// So this gate asks the question where it can be answered: in a browser, over
// the rendered story catalog, with the tokens resolved. jsdom cannot stand in —
// it does not resolve `var()`, so `padding: var(--space-5) 0` and
// `padding: var(--space-5) var(--padCard)` both read back as `0` and the fixed
// rule looks exactly like the broken one.
//
// WHAT COUNTS AS THE DEFECT: an element that draws a box a reader can see the
// edge of, holding text laid directly on that ground, with no inline padding
// between the two. Each exclusion below is one measured false positive, and
// none is a skip-list — they are all properties of the element under the
// question, so a new component gets them for free:
//
//   - a fill matching what it sits on draws no edge (a table cell repeating its
//     card's ground is not a box);
//   - one hairline is a separator, not a box — a border counts all the way round;
//   - centred content is centred, not flush (an avatar's initials);
//   - an inline box takes no inline padding by rights (a <mark> highlight);
//   - text a descendant already stands off is that descendant's business.
//
// UNDER-RECOGNITION IS THE FAILURE MODE THAT MATTERS. A sweep that quietly
// looked at less than it thought reports PASS and there is no failing assertion
// to notice, so a story this gate could not read is a FAILURE, named. Two
// viewports, because the table collapses to cards below 720px and half the
// product's boxes only exist on one side of that line. The walk is rooted at
// every non-Storybook child of <body> rather than at #storybook-root: a story
// whose subject portals itself — a drawer, an overlay — leaves that root empty,
// and rooting there read 29 stories as blank and passed them.
//
// Usage: node frontend/scripts/check-edge-padding.mjs [--filter <substring>]
//   --filter  only stories whose id contains this substring (an inner loop;
//             the gate itself always sweeps the whole catalog)
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
  buildStaticStorybook,
  loadPlaywright,
  readStoryIndex,
  serveStaticStorybook,
} from "./lib/storybook-harness.mjs";

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const staticDir = join(repoRoot, "frontend/storybook-static");

const filterAt = process.argv.indexOf("--filter");
const filter = filterAt === -1 ? null : process.argv[filterAt + 1];

// Wide is the desk; narrow is below the 720px line where `.lt-table` stops
// being a table and becomes a stack of cards. A box that only exists on one
// side of that line is only reachable from that side.
const VIEWPORTS = [
  { tag: "wide", width: 1280, height: 900 },
  { tag: "narrow", width: 420, height: 900 },
];
// Enough parallelism to make the sweep bearable without starving the renders of
// the CPU they need to settle.
const WORKERS = 4;
// A story that has not settled by now has not been slowed down, it is stuck.
const RENDER_BUDGET_MS = 15_000;
// Separate from the settle budget, and generous: a slow fetch off a local file
// server is a fact about the machine, never a finding about the stylesheet.
const NAVIGATION_BUDGET_MS = 30_000;
// Far enough apart that two identical readings mean the layout stopped, rather
// than that both landed inside one animation frame.
const SETTLE_POLL_MS = 300;
// Sub-pixel: a box whose text sits within a pixel of its edge is flush, and
// rounding must not be what decides a verdict.
const FLUSH_PX = 2;

// Runs in the page. Returns the boxes whose text is printed on their own edge,
// or rendered:false when nothing of the story's own was on the page to read.
function probeFlushText(flushPx) {
  const px = (value) => Number.parseFloat(value) || 0;
  const opaque = (colour) =>
    colour &&
    colour !== "transparent" &&
    !/rgba\([^)]*,\s*0\s*\)$/.test(colour);
  // What belongs to the STORY: the canvas root, plus anything the story
  // portalled straight onto the body — a drawer, an overlay, a modal. Storybook
  // ships the rest: the `sb-` preparing/no-preview/error wrappers and the addon
  // roots, which all carry a `storybook-` id. Derived rather than listed, so an
  // addon added later is still not mistaken for the story's own markup; the one
  // `storybook-` id that IS the story is named here because it is Storybook's
  // contract with every renderer, not an addon's detail.
  const painted = (el) =>
    el.children.length > 0 || el.textContent.trim() !== "";
  const roots = [...document.body.children].filter((el) => {
    if (el.tagName === "SCRIPT" || el.tagName === "STYLE") return false;
    if (el.id === "storybook-root") return painted(el);
    if (/^storybook-/.test(el.id)) return false;
    if (/^sb-/.test(String(el.className).split(" ")[0])) return false;
    return painted(el);
  });
  if (roots.length === 0) return { rendered: false, hits: [] };

  // What this element visually sits ON: the nearest ancestor that actually
  // paints. A fill identical to it draws no edge.
  const groundUnder = (el) => {
    for (let parent = el.parentElement; parent; parent = parent.parentElement) {
      const colour = getComputedStyle(parent).backgroundColor;
      if (opaque(colour)) return colour;
    }
    return null;
  };

  const centred = (style) =>
    style.textAlign === "center" ||
    (/flex|grid/.test(style.display) &&
      /center|space-around|space-evenly/.test(style.justifyContent));

  const hits = [];
  for (const root of roots) {
    for (const el of [root, ...root.querySelectorAll("*")]) {
      const style = getComputedStyle(el);
      if (
        style.display === "none" ||
        style.display === "inline" ||
        style.visibility === "hidden" ||
        style.opacity === "0"
      ) {
        continue;
      }
      const box = el.getBoundingClientRect();
      // Smaller than this is a rule, a dot or a spacer, not a box holding prose.
      if (box.width < 8 || box.height < 8) continue;

      const framed = ["Top", "Right", "Bottom", "Left"].every(
        (side) =>
          style[`border${side}Style`] !== "none" &&
          px(style[`border${side}Width`]) > 0,
      );
      const filled =
        opaque(style.backgroundColor) &&
        style.backgroundColor !== groundUnder(el);
      if (!filled && !framed) continue;
      // Each inline edge is asked about separately. Reading only the left one
      // is a blind spot of exactly the shape this gate exists to close: a rule
      // that pays on the left and nothing on the right always shows a wide left
      // gap, so the element was dropped before its flush right edge was ever
      // looked at, and the sweep reported PASS over it.
      const askLeft = px(style.paddingLeft) === 0;
      const askRight = px(style.paddingRight) === 0;
      if (!askLeft && !askRight) continue;
      if (centred(style)) continue;

      const range = document.createRange();
      const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
      let node;
      let nearest = null;
      while ((node = walker.nextNode()) !== null) {
        if (node.textContent.trim() === "") continue;
        // Text a descendant already stands off on a given side is that
        // descendant's business — on that side only.
        let insetLeft = false;
        let insetRight = false;
        for (
          let up = node.parentElement;
          up && up !== el;
          up = up.parentElement
        ) {
          const upStyle = getComputedStyle(up);
          if (px(upStyle.paddingLeft) > 0 || px(upStyle.marginLeft) > 0) {
            insetLeft = true;
          }
          if (px(upStyle.paddingRight) > 0 || px(upStyle.marginRight) > 0) {
            insetRight = true;
          }
        }
        range.selectNodeContents(node);
        const textBox = range.getBoundingClientRect();
        if (textBox.width < 1) continue;
        const edges = [];
        if (askLeft && !insetLeft) {
          edges.push({
            side: "left",
            gap: textBox.left - (box.left + px(style.borderLeftWidth)),
          });
        }
        if (askRight && !insetRight) {
          edges.push({
            side: "right",
            gap: box.right - px(style.borderRightWidth) - textBox.right,
          });
        }
        for (const edge of edges) {
          if (nearest === null || edge.gap < nearest.gap) {
            nearest = {
              gap: edge.gap,
              side: edge.side,
              text: node.textContent.trim(),
            };
          }
        }
      }
      if (nearest === null || nearest.gap > flushPx) continue;

      hits.push({
        selector:
          typeof el.className === "string" && el.className.trim() !== ""
            ? `.${el.className.trim().split(/\s+/).join(".")}`
            : `<${el.tagName.toLowerCase()}>`,
        gap: Math.round(nearest.gap * 10) / 10,
        side: nearest.side,
        text: nearest.text.slice(0, 60),
      });
    }
  }
  return { rendered: true, hits };
}

// The reading of a story that has stopped moving, or null if it never stopped.
//
// The first painted frame is not the verdict, and taking it for one is how this
// gate first reported three panels whose headings are correctly inset: a story
// that paints a pending state before its content lands is momentarily a card
// with a placeholder line against its edge. So a reading counts only once the
// SAME one comes back twice in a row — a condition (the layout has settled),
// not a sleep long enough for the slowest story on an unloaded machine.
async function readSettled(page) {
  const deadline = Date.now() + RENDER_BUDGET_MS;
  let previous = null;
  while (Date.now() < deadline) {
    const probe = await page.evaluate(
      (flushPx) => window.__probeFlushText(flushPx),
      FLUSH_PX,
    );
    if (probe.rendered) {
      const signature = JSON.stringify(probe.hits);
      if (signature === previous) return probe;
      previous = signature;
    }
    await page.waitForTimeout(SETTLE_POLL_MS);
  }
  return null;
}

// A page at one viewport with the probe already in it. The probe is injected
// rather than passed to evaluate() each time so the settle loop can POLL it,
// instead of guessing a sleep long enough for the slowest story.
async function probePage(browser, viewport) {
  const context = await browser.newContext({
    viewport: { width: viewport.width, height: viewport.height },
  });
  await context.addInitScript(
    `window.__probeFlushText = ${probeFlushText.toString()};`,
  );
  const page = await context.newPage();
  // A story that THROWS is fe-uat's finding to report, not this gate's. Here it
  // simply paints nothing of its own, which the caller already treats as a
  // story it could not read.
  page.on("pageerror", () => {});
  return page;
}

// One story at one viewport: its findings, or null if it could not be read.
async function readStory(page, port, storyId, viewportTag) {
  try {
    await page.goto(
      `http://localhost:${port}/iframe.html?id=${encodeURIComponent(storyId)}&viewMode=story`,
      { waitUntil: "load", timeout: NAVIGATION_BUDGET_MS },
    );
    const result = await readSettled(page);
    if (result === null) return null;
    return result.hits.map((hit) => ({
      story: storyId,
      viewport: viewportTag,
      ...hit,
    }));
  } catch {
    return null;
  }
}

async function sweepViewport(browser, port, stories, viewport) {
  const findings = [];
  const unread = [];
  const lanes = Array.from({ length: WORKERS }, (_, lane) =>
    stories.filter((_, index) => index % WORKERS === lane),
  );

  await Promise.all(
    lanes.map(async (lane) => {
      const page = await probePage(browser, viewport);
      for (const story of lane) {
        const hits = await readStory(page, port, story.id, viewport.tag);
        if (hits === null)
          unread.push({ story: story.id, viewport: viewport.tag });
        else findings.push(...hits);
      }
      await page.context().close();
    }),
  );
  return { findings, unread };
}

// Re-read the misses ONE AT A TIME, with nothing else in flight.
//
// A story rendering beside three others can be starved of the CPU it needs to
// settle, and a verdict that depends on how busy the machine was is not a
// verdict. Measured on the story that first failed this way: it stops mutating
// 1.2s in and each probe costs 2ms, nowhere near the budget it was given — it
// was not slow, it was crowded. So each miss gets exactly one quiet re-read,
// which is what separates a story this gate cannot read from a loaded runner.
// A miss that survives it is real, and is reported.
async function reReadAlone(browser, port, misses) {
  const findings = [];
  const unread = [];
  for (const miss of misses) {
    const viewport = VIEWPORTS.find((v) => v.tag === miss.viewport);
    const page = await probePage(browser, viewport);
    const hits = await readStory(page, port, miss.story, miss.viewport);
    if (hits === null) unread.push(miss);
    else findings.push(...hits);
    await page.context().close();
  }
  return { findings, unread };
}

async function main() {
  buildStaticStorybook(repoRoot, staticDir, { force: true });
  const all = readStoryIndex(staticDir).filter((s) => s.type !== "docs");
  const stories = filter ? all.filter((s) => s.id.includes(filter)) : all;
  if (stories.length === 0) {
    console.error(
      filter
        ? `No story id contains ${JSON.stringify(filter)}.`
        : "The built Storybook registered no stories.",
    );
    process.exit(1);
  }

  const { port, close } = await serveStaticStorybook(staticDir);
  const { chromium } = await loadPlaywright();
  const browser = await chromium.launch();

  const findings = [];
  const unread = [];
  // The swept-pair count when the run failed as a HARNESS rather than as a
  // sweep, zero otherwise.
  let harnessFailure = 0;
  try {
    for (const viewport of VIEWPORTS) {
      console.log(
        `Reading ${stories.length} stories at ${viewport.width}px (${viewport.tag})…`,
      );
      const pass = await sweepViewport(browser, port, stories, viewport);
      findings.push(...pass.findings);
      unread.push(...pass.unread);
    }
    if (unread.length > 0) {
      // A handful of misses is a story, and worth the quiet re-read below. Most
      // of the catalog missing is the HARNESS — the server gone, Chromium
      // unable to start, a build serving no iframe — and re-reading thousands
      // of pairs one at a time at up to RENDER_BUDGET_MS each spends hours
      // arriving at the verdict this count already gives.
      const sweptPairs = stories.length * VIEWPORTS.length;
      // Reported after the browser and the server are shut down, rather than
      // exiting here: process.exit skips the finally below and leaves Chromium
      // orphaned on a machine whose harness is already unwell.
      harnessFailure = unread.length > sweptPairs / 2 ? sweptPairs : 0;
      if (harnessFailure === 0) {
        console.log(
          `Re-reading ${unread.length} story/viewport pair(s) one at a time…`,
        );
        const quiet = await reReadAlone(browser, port, unread.splice(0));
        findings.push(...quiet.findings);
        unread.push(...quiet.unread);
      }
    }
  } finally {
    await browser.close();
    close();
  }

  if (harnessFailure > 0) {
    console.error(
      `\n${unread.length} of ${harnessFailure} story/viewport pairs could not be read.\n` +
        "That is the harness rather than the stylesheets — nothing rendered to\n" +
        "measure. Fix the run and try again; this says nothing about the CSS.",
    );
    process.exit(1);
  }

  // One CSS defect shows up in every story that renders the component, so the
  // report is grouped by what to go and fix rather than by where it was seen.
  const byBox = new Map();
  for (const finding of findings) {
    const group = byBox.get(finding.selector) ?? {
      stories: new Set(),
      viewports: new Set(),
      sample: finding,
    };
    group.stories.add(finding.story);
    group.viewports.add(finding.viewport);
    byBox.set(finding.selector, group);
  }

  console.log(
    `\nRead ${stories.length} stories x ${VIEWPORTS.length} viewports.`,
  );
  if (byBox.size > 0) {
    console.error(
      `\n${byBox.size} box(es) print text against their own edge:\n`,
    );
    for (const [selector, group] of [...byBox.entries()].sort(
      (a, b) => b[1].stories.size - a[1].stories.size,
    )) {
      console.error(
        `  ${selector}  — ${group.stories.size} stor${group.stories.size === 1 ? "y" : "ies"} [${[...group.viewports].join(", ")}]`,
      );
      console.error(
        `      ${JSON.stringify(group.sample.text)} sits ${group.sample.gap}px from the ${group.sample.side} edge`,
      );
      console.error(`      e.g. ${[...group.stories][0]}`);
    }
    console.error(
      "\nThe box needs inline padding. Where it is a design-system surface, name",
      "\nthe role (var(--padCard) / var(--padPanel)) rather than a rung, so the",
      "\ninset moves when that surface is retuned.",
    );
  }
  if (unread.length > 0) {
    // Not a warning. A sweep that could not read a story has not passed it, and
    // reporting it as clean is the one way this gate could be wrong in silence.
    console.error(
      `\n${unread.length} story/viewport pair(s) could not be read:`,
    );
    for (const miss of unread.slice(0, 20)) {
      console.error(`  ${miss.story} [${miss.viewport}]`);
    }
    if (unread.length > 20) console.error(`  …and ${unread.length - 20} more`);
  }
  if (byBox.size > 0 || unread.length > 0) process.exit(1);
  console.log("No text printed against a box edge.");
}

await main();
