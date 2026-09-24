/** @vitest-environment happy-dom */

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { de } from "../i18n/de";
import { en } from "../i18n/en";
import { AgentRail } from "./agentrail";
import { type GrantSpec, meFixture } from "./mefixture";

// THE PANEL'S SHAPE, which is the half of it no other suite asserts.
//
// The cases beside this one prove what the panel SAYS — that a count nobody
// read is never printed, that a licence pill leads to the page that repairs it.
// These prove the order and the structure it says it in, because that is what
// went wrong: one heading level for four different kinds of thing, the state in
// the Core's own machine vocabulary, the live sentence promoted to the panel's
// title, and the month's figure printed twice.
//
// They live apart from agentrail.test.tsx because that file is at the size a
// test file may grow to.

type Occurrence = Readonly<Record<string, unknown>>;

const NOW = Date.now();

/** A settled occurrence, as the recap reads them. */
function settled(minutesAgo: number, over: Occurrence): Occurrence {
  return {
    id: `run-${String(over.kind)}-${String(over.state)}`,
    kind: "morning_brief",
    started_at: new Date(NOW - (minutesAgo + 1) * 60_000).toISOString(),
    finished_at: new Date(NOW - minutesAgo * 60_000).toISOString(),
    ...over,
  };
}

/** A month with one priced line, so the figure is $1.20 and not a round one. */
const PRICED_USAGE = {
  days: [
    {
      date: "2026-08-01",
      tasks: [
        {
          task: "enrich",
          tier: "cheap_cloud",
          calls: 2,
          tokens_in: 100,
          tokens_out: 40,
          cost_est_minor: 120,
        },
      ],
    },
  ],
  budget: { monthly_tokens: 0, spent_tokens: 0, band: "normal" },
};

const AI_CALL = {
  id: "019f7e65-fbf7-7114-b114-40af4af63ae8",
  occurred_at: "2026-07-20T10:00:00Z",
  task: "capture_classify",
  tier: "cheap_cloud",
  provider: "gemini",
  model_id: "configured",
  served_model: "served",
  calls_attempted: 1,
  tokens_in: 10,
  tokens_out: 5,
  reasoning_tokens: 0,
  cached_tokens: 0,
  latency_ms: 400,
  has_payload: false,
};

/** The administrator's seat: the only one the month's figure is served to. */
const ADMIN: GrantSpec = { ai_diagnostics: ["read"], license: ["read"] };

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

type Answers = Readonly<{
  running?: readonly Occurrence[];
  /** Absent means the body carries no `live_total` at all. */
  liveTotal?: number;
  recent?: readonly Occurrence[];
  priced?: boolean;
  calls?: readonly unknown[];
}>;

// Every read the panel makes, answered with a healthy installation: these cases
// are about the shape of the report, so nothing here should be in a state of
// its own unless the case put it there.
function stubApi(answers: Answers) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const pathname = new URL(request.url).pathname;
      if (pathname.endsWith("/me")) {
        return jsonResponse(meFixture({ allow: ADMIN }));
      }
      if (pathname.endsWith("/assistant/profile")) {
        return jsonResponse({
          name: "Margince",
          kind: "ai",
          state: "configured",
          inference_mode: "cloud",
          providers: ["anthropic"],
        });
      }
      if (pathname.endsWith("/installation/license")) {
        return jsonResponse({
          state: "valid",
          seats_used: 1,
          over_limit: false,
          checked_at: "2026-08-01T09:00:00Z",
        });
      }
      if (pathname.endsWith("/me/ai-activity")) {
        return jsonResponse({
          running: answers.running ?? [],
          recent: answers.recent ?? [],
          faults: [],
          ...(answers.liveTotal === undefined
            ? {}
            : { live_total: answers.liveTotal }),
        });
      }
      if (pathname.endsWith("/connectors")) {
        return jsonResponse({ data: [] });
      }
      if (pathname.endsWith("/ai/calls")) {
        return jsonResponse({ data: answers.calls ?? [], tasks: [] });
      }
      if (pathname.endsWith("/ai/usage")) {
        return jsonResponse(
          answers.priced
            ? PRICED_USAGE
            : {
                days: [],
                budget: { monthly_tokens: 0, spent_tokens: 0, band: "normal" },
              },
        );
      }
      return jsonResponse({
        data: [],
        page: { has_more: false, next_cursor: null },
      });
    }),
  );
}

async function openPanel(answers: Answers = {}, locale: "en" | "de" = "en") {
  stubApi(answers);
  const user = userEvent.setup();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const { container } = render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>
        <AgentRail route={{ screen: "deals" }} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  const trigger = container.querySelector(".arhit");
  if (!trigger) throw new Error("no .arhit trigger in the rendered tree");
  await user.click(trigger);
  const opened = document.querySelector(".arpanel");
  if (!opened) throw new Error("no .arpanel on the document after the click");
  return opened;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the agent panel's hierarchy", () => {
  // ONE title, and it names the agent. The head used to lead with whatever
  // sentence the rail was showing, which is a heading that renames the region
  // on every poll — and on a resting installation it was a product tip.
  it("titles the panel with the agent's name and states the live line under it", async () => {
    const opened = await openPanel();
    const titles = [...opened.querySelectorAll("h2")].map(
      (el) => el.textContent,
    );
    expect(titles).toEqual(["Margince"]);
    // The sentence itself rotates every few seconds, so what is asserted is
    // where it stands: in the head, under the title, and not as it.
    const head = [...(opened.querySelector(".arphead")?.children ?? [])];
    const titleAt = head.findIndex((part) => part.tagName === "H2");
    const sayingAt = head.findIndex((part) =>
      part.classList.contains("arpsaying"),
    );
    expect(titleAt).toBeGreaterThan(-1);
    expect(sayingAt).toBeGreaterThan(titleAt);
    expect(head[sayingAt]?.textContent).toBeTruthy();
  });

  // Every section is one level under that title and no level is skipped, which
  // is what the panel got wrong in both directions before: h4s that skipped two
  // levels, then h2s that made four sections siblings of the page's own title.
  it("gives every section a heading one level under the title", async () => {
    const opened = await openPanel();
    const sections = [...opened.querySelectorAll("section.arsect")];
    expect(sections.length).toBeGreaterThan(1);
    for (const section of sections) {
      const heading = section.querySelector("h3");
      expect(heading?.textContent).toBeTruthy();
      // A named region, named by the heading a reader already sees.
      expect(section.getAttribute("aria-labelledby")).toBe(heading?.id);
    }
    expect(opened.querySelectorAll("h4, h5, h6")).toHaveLength(0);
  });

  // The verb belongs to the section, not to its title: inside the heading it
  // wore the heading's weight and read as a second title beside the first.
  it("puts the full-log verb beside the recap's title, not inside it", async () => {
    const opened = await openPanel();
    const head = [...opened.querySelectorAll(".arsecthead")].find((one) =>
      one.textContent?.includes(en["agent.panel.recent"]),
    );
    expect(head?.querySelector("h3")?.textContent).toBe(
      en["agent.panel.recent"],
    );
    expect(head?.querySelector("a.link-button")?.textContent).toBe(
      en["agent.panel.fullLog"],
    );
  });

  // The Core's vocabulary is five machine words and this is the one place they
  // are written out, so they are written out in the reader's language.
  it("says how the agent is in the reader's word, not the Core's", async () => {
    const opened = await openPanel();
    expect(opened.querySelector(".arpstate")?.textContent).toBe(
      en["agent.state.idle"],
    );
    cleanup();
    const german = await openPanel({}, "de");
    expect(german.querySelector(".arpstate")?.textContent).toBe(
      de["agent.state.idle"],
    );
    expect(de["agent.state.idle"]).not.toBe(en["agent.state.idle"]);
  });

  // ONE figure. It stood in the head AND in the runtime rows, which invites a
  // reader to check whether the two agree — and one of them was drawn from a
  // second call of the same hook.
  it("says the month's cost once, in the head", async () => {
    const opened = await openPanel({ priced: true });
    await waitFor(() =>
      expect(opened.querySelector(".arpmoney")?.textContent).toContain("$1.20"),
    );
    expect(opened.textContent?.match(/\$1\.20/g)).toHaveLength(1);
  });

  // The claim is the last thing on the surface because it is the only line no
  // read produced: a standing promise rather than news. AC-shell-8 holds that
  // it is SAID; this holds where.
  it("ends with the privacy claim", async () => {
    const opened = await openPanel();
    expect(opened.lastElementChild?.className).toContain("arclaim");
  });
});

describe("the agent panel's report", () => {
  // The mark used to carry the orb's CURRENT tone at a fading opacity, so a
  // brief that failed at four in the morning was drawn in the green of a quiet
  // afternoon. It says how that one went, in the five states.
  it("marks each recap row with how that occurrence went", async () => {
    const opened = await openPanel({
      recent: [
        settled(12, { state: "failed", kind: "morning_brief" }),
        settled(40, { state: "done", kind: "site_read" }),
      ],
    });
    await waitFor(() =>
      expect(opened.querySelectorAll(".armark")).toHaveLength(2),
    );
    const tones = [...opened.querySelectorAll(".armark")].map((mark) =>
      mark.getAttribute("data-tone"),
    );
    expect(tones).toEqual(["danger", "success"]);
  });

  // Four unrelated facts were one wrapped line of bold-and-plain fragments,
  // with nothing saying which word named which. A term and its value is what
  // they are, so a definition list is what they are drawn as.
  it("draws what it is standing on as terms and values", async () => {
    const opened = await openPanel({ calls: [AI_CALL] });
    await waitFor(() =>
      expect(opened.querySelector(".arfacts")?.tagName).toBe("DL"),
    );
    const facts = opened.querySelector(".arfacts");
    const terms = [...(facts?.querySelectorAll("dt") ?? [])].map(
      (el) => el.textContent,
    );
    expect(terms).toContain(en["agent.fact.model"]);
    // Every term answered: a dt with no dd is a label for nothing.
    expect(facts?.querySelectorAll("dd")).toHaveLength(terms.length);
    await waitFor(() =>
      expect(facts?.textContent).toContain(
        `${AI_CALL.provider}/${AI_CALL.served_model}`,
      ),
    );
  });

  // A quiet line rather than a dashed plate: the dashes read as a tile whose
  // number failed to load, which is the opposite of what an all-clear says.
  //
  // And its OWN sentence. The head's resting line says "Nothing needs attention" on
  // exactly the installation this section is empty on, so the two stood on one
  // panel saying the same four words.
  it("says the all-clear as a plain line, undashed and in its own words", async () => {
    const opened = await openPanel();
    await waitFor(() =>
      expect(opened.querySelector(".arnone")?.textContent).toBe(
        en["agent.panel.nothingWaiting"],
      ),
    );
    expect(opened.querySelector(".arnone")?.className).toContain("t-caption");
    expect(en["agent.panel.nothingWaiting"]).not.toBe(
      en["agent.line.allClear"],
    );
  });
});

// The live total counts work of every kind, and the rail narrates only some of
// them. So the light may pulse for work it cannot name, and the panel admits
// that work without offering a row to read — while a run it CAN name still
// names itself, because the count never outranks the feed.
describe("work the rail cannot name", () => {
  const coreState = () =>
    document.querySelector(".arblock")?.getAttribute("data-core-state");
  const runningSection = (opened: Element) =>
    [...opened.querySelectorAll("section.arsect")].find(
      (section) =>
        section.querySelector("h3")?.textContent ===
        en["agent.panel.runningNow"],
    );

  it("pulses with the generic line and a caption, listing no run", async () => {
    const opened = await openPanel({ liveTotal: 1 });
    await waitFor(() => expect(coreState()).toBe("working"));
    // The generic state word, since no cause travels with it.
    expect(opened.querySelector(".arpsaying")?.textContent).toBe(
      en["agent.state.working"],
    );
    const section = runningSection(opened);
    expect(section?.querySelector(".arempty")?.textContent).toBe(
      en["agent.panel.unnamedLive"],
    );
    expect(section?.querySelector(".arempty")?.className).toContain(
      "t-caption",
    );
    expect(opened.querySelectorAll(".arrun")).toHaveLength(0);
  });

  it("names the run it can narrate, however large the total", async () => {
    const opened = await openPanel({
      running: [
        {
          id: "run-live",
          kind: "morning_brief",
          state: "running",
          started_at: new Date(NOW - 60_000).toISOString(),
        },
      ],
      liveTotal: 5,
    });
    await waitFor(() =>
      expect(opened.querySelectorAll(".arrun")).toHaveLength(1),
    );
    expect(opened.querySelector(".arpsaying")?.textContent).toBe(
      opened.querySelector(".arrunline")?.textContent,
    );
    expect(runningSection(opened)?.querySelector(".arempty")).toBeNull();
  });

  it("captions the background work beside a stalled row", async () => {
    const opened = await openPanel({
      running: [
        {
          id: "run-stalled",
          kind: "morning_brief",
          state: "stalled",
          started_at: new Date(NOW - 60_000).toISOString(),
        },
      ],
      liveTotal: 1,
    });
    await waitFor(() =>
      expect(opened.querySelectorAll(".arrun")).toHaveLength(1),
    );
    expect(runningSection(opened)?.querySelector(".arempty")?.textContent).toBe(
      en["agent.panel.unnamedLive"],
    );
  });

  it("keeps the caption out while a named row is live", async () => {
    const opened = await openPanel({
      running: [
        {
          id: "run-live",
          kind: "morning_brief",
          state: "running",
          started_at: new Date(NOW - 60_000).toISOString(),
        },
      ],
      liveTotal: 2,
    });
    await waitFor(() =>
      expect(opened.querySelectorAll(".arrun")).toHaveLength(1),
    );
    expect(runningSection(opened)?.querySelector(".arempty")).toBeNull();
  });

  it("rests when the total is zero and nothing is listed", async () => {
    const opened = await openPanel({
      liveTotal: 0,
      recent: [settled(12, { state: "done", kind: "morning_brief" })],
    });
    // The recap row is the feed's own, so the feed has answered once it shows.
    await waitFor(() =>
      expect(opened.querySelectorAll(".aritem")).toHaveLength(1),
    );
    expect(coreState()).toBe("idle");
    expect(runningSection(opened)).toBeUndefined();
    expect(opened.textContent).not.toContain(en["agent.panel.unnamedLive"]);
  });
});
