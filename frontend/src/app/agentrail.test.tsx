/** @vitest-environment happy-dom */
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { QueryClient, useMutation } from "@tanstack/react-query";
import { act, cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { endModelCall, modelCallsInFlight } from "../api/model-inflight";
import type { components } from "../api/schema";
import { en } from "../i18n/en";
import { meFixture } from "../testing/mefixture";
import { stubPhoneViewport } from "../testing/shellharness";
import {
  AGENT_EDGE_STILL,
  clearAgentEdge,
  currentAgentEdge,
} from "./agent-edge-signal";
import {
  block,
  CONNECTED,
  emptyPage,
  jsonResponse,
  LICENSE,
  OPERATOR,
  openPanel,
  PROFILE,
  panel,
  ROUTE,
  render,
  settlesOnLine,
  stubAgentRailApi,
} from "./agentrail.testkit";
import { VOCABULARY } from "./agentrail-copy";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
  // The in-flight count is module state. It is reset here the way
  // clearAgentEdge resets its sibling signal. Otherwise a case failing between
  // begin and end would start every later case in this file at "working".
  while (modelCallsInFlight() > 0) {
    endModelCall();
  }
  // The frame case spies on Element.prototype.getBoundingClientRect, which every
  // later case in this file shares. Restored here rather than there: a spy left
  // on a prototype is the kind of leak whose failure names a case that is fine.
  vi.restoreAllMocks();
  vi.useRealTimers();
});

type Candidate = components["schemas"]["DedupeCandidate"];

type Approval = components["schemas"]["Approval"];

type AiCallSummary = components["schemas"]["AiCallSummary"];

const CANDIDATE = (id: string): Candidate => ({
  id,
  entity_type: "company",
  left_id: "o-1",
  right_id: "o-2",
  confidence: 0.91,
  evidence: [],
  // Fed only to the test that proves the rail says nothing of the queue, so
  // the authority is granted rather than left as a second reason for silence.
  can_decide: true,
  status: "open",
  created_at: "2026-08-01T09:00:00Z",
});

const APPROVAL = (id: string): Approval => ({
  id,
  kind: "send_email",
  status: "pending",
  proposed_by: "agent:runner",
  created_at: "2026-08-01T09:00:00Z",
});

const AI_CALL: AiCallSummary = {
  id: "019f7e65-fbf7-7114-b114-40af4af63ae8",
  occurred_at: "2026-07-20T10:00:00Z",
  kind: "completion",
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
  cache_hit: false,
  degraded: false,
  has_payload: false,
  decision_attempted: false,
  outcome: "answered",
};

/** A month with one priced line and one the server could not price. */
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
        {
          task: "summarize",
          tier: "cheap_cloud",
          calls: 1,
          tokens_in: 30,
          tokens_out: 10,
        },
      ],
    },
  ],
  budget: { monthly_tokens: 0, spent_tokens: 0, band: "normal" },
};

/** A pending write, held open so `useIsMutating` reports it for the life of
 *  the test — the only way to reach `working` from outside the component. */
function PendingWrite() {
  const write = useMutation({
    mutationFn: () => new Promise<void>(() => {}),
  });
  // Fired once, on mount: a mutation held open for the life of the test is
  // the only way to reach `working` from outside the component under test.
  // biome-ignore lint/correctness/useExhaustiveDependencies: mutate is stable for the life of this hook; listing it would refire the pending write on every render instead of holding one open.
  useEffect(() => {
    write.mutate();
  }, []);
  return null;
}

describe("AgentRail", () => {
  // The resting line is a rotation of TRUE readings, and an installation with a
  // clean queue has only one true thing to say. Which is the point of the case:
  // a resting surface says nothing rather than inventing something to report.
  it("is idle and says nothing needs you when every source is connected and nothing is queued", async () => {
    stubAgentRailApi();
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("idle"),
    );
    await settlesOnLine(container, en["agent.line.allClear"]);
  });

  it("goes to error and names the source when a connector cannot reach it", async () => {
    stubAgentRailApi({
      connectors: () =>
        jsonResponse({
          data: [
            {
              ...CONNECTED,
              status: "reauth_required",
              account_label: "sales@old-crm.example",
            },
          ],
        }),
    });
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("error"),
    );
    await waitFor(
      () =>
        expect(container.querySelector(".arline")?.textContent).toContain(
          "sales@old-crm.example",
        ),
      { timeout: 3000 },
    );
  });

  it("goes to error when no AI model is configured", async () => {
    stubAgentRailApi({ profile: () => jsonResponse(PROFILE("unconfigured")) });
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("error"),
    );
    await settlesOnLine(container, en["agent.fact.noModel"]);
  });

  // Development is not a fault — the runtime answers, every answer it gives
  // is invented, and that is a standing fact said calmly in the runtime row
  // rather than a state the section treats as broken.
  it("stays idle on the development AI posture and names it in the runtime row, not as a fault", async () => {
    const user = userEvent.setup();
    stubAgentRailApi({ profile: () => jsonResponse(PROFILE("development")) });
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("idle"),
    );
    await openPanel(user, container);
    await waitFor(() => {
      const runtime = panel().querySelector(".armeta")?.textContent ?? "";
      expect(runtime).toContain("Development AI");
      expect(runtime).toContain("Offline development mode");
    });
  });

  // A seat without `license:read` gets `undefined`, not a fault: a read this
  // seat may not make is none of its business, and its panel names no licence.
  it("gives no licence row when the seat may not read the licence", async () => {
    const user = userEvent.setup();
    stubAgentRailApi({
      me: () =>
        jsonResponse(meFixture({ allow: { ai_diagnostics: ["read"] } })),
      license: () => jsonResponse(LICENSE("absent")),
    });
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("idle"),
    );
    await openPanel(user, container);
    await waitFor(() => {
      expect(panel().querySelector(".armeta")?.textContent).not.toContain(
        "No license",
      );
    });
  });

  // A dropped request is the tool's own problem for that one call, and does
  // not colour the orb. A corner that flashed red on a flaky connection is the
  // failure this derivation was rebuilt to avoid. Nor does the line say that
  // nothing needs attention: nobody read which sources are reachable.
  it("does not colour the orb for a transient tool failure, and claims no all-clear", async () => {
    stubAgentRailApi({
      connectors: () =>
        jsonResponse(
          { code: "internal", title: "internal error", status: 500 },
          500,
        ),
    });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const { container } = render(ROUTE, { client });
    // A connectors read still in flight also reads as idle, so the state is
    // asserted only once the 500 has landed in the cache as an error.
    await waitFor(() =>
      expect(client.getQueryState(["connectors"])?.status).toBe("error"),
    );
    await settlesOnLine(container, en["agent.line.sourcesUnknown"]);
    expect(block(container).getAttribute("data-core-state")).toBe("idle");
  });

  // The Core speaks the AGENT's vocabulary, so only the agent may move it. A
  // read in flight is the browser fetching for the reader who asked, and it used
  // to put the orb in `ingest` — which made the one state named for the agent
  // taking something in the one state the agent could never cause.
  it("does not move the Core for the tool's own reads", async () => {
    vi.useFakeTimers();
    stubAgentRailApi({ connectors: () => new Promise<Response>(() => {}) });
    const { container } = render(ROUTE);
    // Past both the BUSY_ON_MS/BUSY_OFF_MS steady windows (app/activity.ts)
    // and the ticker's own LINGER_MS, so what is asserted below is the settled
    // reading and not a window either of those two is still inside.
    await act(() => vi.advanceTimersByTimeAsync(2000));
    expect(block(container).getAttribute("data-core-state")).toBe("idle");
  });

  it("does not move the Core for the reader's own writes", async () => {
    vi.useFakeTimers();
    stubAgentRailApi();
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const { container } = render(ROUTE, {
      client,
      children: <PendingWrite />,
    });
    await act(() => vi.advanceTimersByTimeAsync(2000));
    expect(block(container).getAttribute("data-core-state")).toBe("idle");
  });

  // The count reaches the reader through the LINE and through the rail's own
  // approvals badge, which is a row above this one. The section carries no
  // second badge of its own: the same number twice in one column is a reader
  // checking whether the two agree.
  it("says what is waiting in its line, and badges no count of its own", async () => {
    stubAgentRailApi({
      approvals: () =>
        jsonResponse({
          data: [APPROVAL("a-1"), APPROVAL("a-2"), APPROVAL("a-3")],
          page: emptyPage,
        }),
    });
    const { container } = render(ROUTE);
    await settlesOnLine(
      container,
      en["agent.line.waiting_other"].replace("{count}", "3"),
    );
    expect(block(container).querySelector(".arbadge")).toBeNull();
  });

  // The duplicate queue is not the agent's work. It is a queue the product
  // keeps, decided in its own lane of the Worklist, and it reached this surface
  // in three places at once: a turn in the resting rotation, a tile in the
  // panel, and a second telling of a number the worklist already carries. None
  // of them is here now, and the state was never one of them because a queue is
  // not a fault.
  it("says nothing about the duplicate queue, in the line, the state or the panel", async () => {
    const user = userEvent.setup();
    const fetchMock = stubAgentRailApi({
      dedupe: () =>
        jsonResponse({ data: [CANDIDATE("d-1"), CANDIDATE("d-2")] }),
    });
    const { container } = render(ROUTE);
    await settlesOnLine(container, en["agent.line.allClear"]);
    expect(block(container).getAttribute("data-core-state")).toBe("idle");

    await openPanel(user, container);
    expect(
      screen.queryByRole("link", { name: /Duplicate pairs open/ }),
    ).toBeNull();
    expect(panel().textContent).not.toContain("Duplicate");
    expect(
      fetchMock.mock.calls.some(([request]) =>
        new URL(request.url).pathname.endsWith("/dedupe/candidates"),
      ),
    ).toBe(false);
  });

  // Absence, not zero: a count nobody has computed yet must not be printed —
  // neither as the section's own badge nor as a row in the panel. Asserted
  // before the approvals fetch ever settles, which is the state a status
  // surface has to be honest in.
  it("prints no count and no approvals row while the approvals read has not answered", async () => {
    const user = userEvent.setup();
    stubAgentRailApi({ approvals: () => new Promise<Response>(() => {}) });
    const { container } = render(ROUTE);
    await openPanel(user, container);
    expect(container.querySelector(".arbadge")).toBeNull();
    expect(
      screen.queryByRole("link", {
        name: new RegExp(`^${en["agent.panel.decisions"]}`),
      }),
    ).toBeNull();
  });

  // THREE cases, and the middle one is the whole point: a count nobody has read
  // is not a count of zero. An unread count says nothing at all; a count that
  // ANSWERED zero earns the all-clear.
  it("says nothing about what is waiting until the read answers, and all-clear only when it answered zero", async () => {
    const user = userEvent.setup();
    stubAgentRailApi({ approvals: () => new Promise<Response>(() => {}) });
    const { container } = render(ROUTE);
    await openPanel(user, container);
    // The whole section is absent: its heading, its all-clear and its tile. An
    // unread count is reported by saying nothing, never by an all-clear the
    // panel never read. (The resting LINE above it is a different reading with
    // its own rules and is not what this asserts.)
    expect(panel().textContent).not.toContain(en["agent.panel.needsYou"]);
    expect(
      screen.queryByRole("link", {
        name: new RegExp(`^${en["agent.panel.decisions"]}`),
      }),
    ).toBeNull();
  });

  it("says all-clear once the read answers zero", async () => {
    const user = userEvent.setup();
    stubAgentRailApi();
    const { container } = render(ROUTE);
    await openPanel(user, container);
    // The section's OWN sentence. This used to be scoped to the workspace
    // section by hand, because the resting line in the head said "Nothing needs
    // you" too and a whole-panel match would have passed with the paragraph
    // gone; the two no longer share a sentence, so matching one matches it.
    await waitFor(() =>
      expect(panel().querySelector(".arnone")?.textContent).toBe(
        en["agent.panel.nothingWaiting"],
      ),
    );
    // A real zero is an answer, not a tile: "0 Decisions waiting" is a number
    // nobody has to act on dressed as one somebody does.
    expect(
      screen.queryByRole("link", {
        name: new RegExp(`^${en["agent.panel.decisions"]}`),
      }),
    ).toBeNull();
  });

  // A refusal reads exactly like a read that has not answered — the row must
  // stay absent once the 403 lands too, not just before it.
  it("prints no count and no approvals row when the approvals read is refused", async () => {
    const user = userEvent.setup();
    stubAgentRailApi({
      approvals: () =>
        jsonResponse(
          { code: "permission_denied", title: "denied", status: 403 },
          403,
        ),
    });
    const { container } = render(ROUTE);
    await openPanel(user, container);
    await waitFor(() =>
      expect(
        screen.queryByRole("link", {
          name: new RegExp(`^${en["agent.panel.decisions"]}`),
        }),
      ).toBeNull(),
    );
    expect(container.querySelector(".arbadge")).toBeNull();
  });

  // Where the panel is measured from at phone width, what it spans, and what
  // points at it.
  //
  // There it stands OVER its anchor rather than beside it, and the anchor is the
  // round well the orb sits in — which rises clear of the bar's top edge. Take
  // the measurement from the CELL behind that well and the panel opens across
  // the orb it belongs to; take the notch's x from the middle of the SCREEN and
  // it stops pointing at the thing that was pressed the moment the bar's cells
  // stop being even. Both come off the same measured box, and this is the case
  // that says so: the well and the cell are deliberately given different boxes,
  // and every number below is the well's.
  it("spans the bar and measures the phone panel from the well the orb sits in", async () => {
    const user = userEvent.setup();
    stubPhoneViewport();
    stubAgentRailApi();
    // Real DOMRects rather than object literals cast into the shape: a rect
    // whose `top` was supplied and whose `bottom` was not is a box no element
    // has, and the derived fields would then be whatever the literal happened to
    // spell. The well stands 16px clear of the cell behind it, which is the one
    // difference every assertion below turns on.
    const well = new DOMRect(167, 700, 56, 56);
    const cell = new DOMRect(151, 716, 88, 48);
    const rail = new DOMRect(12, 716, 366, 58);
    const nowhere = new DOMRect(0, 0, 0, 0);
    const boxFor = (element: Element) => {
      if (element.classList.contains("arhit")) {
        return well;
      }
      if (element.classList.contains("arblock")) {
        return cell;
      }
      return element.classList.contains("rail") ? rail : nowhere;
    };
    vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(
      function (this: Element) {
        return boxFor(this);
      },
    );
    const bar = document.createElement("nav");
    bar.className = "rail";
    const { container } = render(ROUTE, { bar: { current: bar } });

    await openPanel(user, container);
    const loose = panel();
    const surface = loose.querySelector<HTMLElement>(".arpanel");
    // 8px of air over the WELL's top edge, not over the cell 16px below it.
    expect(surface?.style.bottom).toBe(`${globalThis.innerHeight - 700 + 8}px`);
    // The bar's own span, edge to edge: the panel and the bar are one object,
    // and a panel inset by a margin of its own reads as a sheet that happened to
    // arrive over it.
    expect(surface?.style.left).toBe("12px");
    expect(surface?.style.right).toBe(`${globalThis.innerWidth - 378}px`);
    // The well's own middle: 167 + 56 / 2.
    expect(loose.getAttribute("style")).toContain("--arCaretX: 195px");
  });

  // Beside the card on a sidebar, and nothing pointing at anything: the panel
  // and the block it came from are side by side and a notch would have no gap to
  // cross. The variable is absent rather than set to a value the sheet ignores.
  it("carries no notch on the sidebar", async () => {
    const user = userEvent.setup();
    stubAgentRailApi();
    const { container } = render(ROUTE);

    await openPanel(user, container);
    expect(panel().getAttribute("style")).not.toContain("--arCaretX");
  });

  it("opens the panel on a click of the section and flips its expanded state", async () => {
    const user = userEvent.setup();
    stubAgentRailApi();
    const { container } = render(ROUTE);
    expect(document.querySelector(".arloose")).toBeNull();
    const trigger = container.querySelector(".artoggle");
    expect(trigger?.getAttribute("aria-expanded")).toBe("false");

    await openPanel(user, container);
    expect(document.querySelector(".arloose")).not.toBeNull();
    expect(
      container.querySelector(".artoggle")?.getAttribute("aria-expanded"),
    ).toBe("true");
  });

  it("says the runtime row is not readable without ai_diagnostics:read", async () => {
    const user = userEvent.setup();
    stubAgentRailApi({
      me: () => jsonResponse(meFixture({ allow: { license: ["read"] } })),
    });
    const { container } = render(ROUTE);
    await openPanel(user, container);
    await waitFor(() =>
      expect(panel().querySelector(".armeta")?.textContent).toContain(
        en["agent.fact.hidden"],
      ),
    );
  });

  it("says nothing has run yet when the seat may read /ai/calls and none has", async () => {
    const user = userEvent.setup();
    stubAgentRailApi();
    const { container } = render(ROUTE);
    await openPanel(user, container);
    await waitFor(() =>
      expect(panel().querySelector(".armeta")?.textContent).toContain(
        en["agent.fact.noCalls"],
      ),
    );
  });

  it("names the served model once a call has actually run", async () => {
    const user = userEvent.setup();
    stubAgentRailApi({
      aiCalls: () =>
        jsonResponse({
          data: [AI_CALL],
          page: emptyPage,
          payload_capture_enabled: false,
          ...{ tasks: [AI_CALL.task], task_options: [{ task: AI_CALL.task }] },
        }),
    });
    const { container } = render(ROUTE);
    await openPanel(user, container);
    await waitFor(() =>
      expect(panel().querySelector(".armeta")?.textContent).toContain(
        `${AI_CALL.provider}/${AI_CALL.served_model}`,
      ),
    );
  });

  // No line carries a priced figure when nothing in the month was priced: the
  // difference between "this cost nothing" and "nobody knows what this cost"
  // is the whole point of the row.
  it("shows no spend figure when nothing in the month was priced", async () => {
    const user = userEvent.setup();
    stubAgentRailApi();
    const { container } = render(ROUTE);
    await openPanel(user, container);
    await waitFor(() =>
      expect(panel().querySelector(".armeta")?.textContent).toContain(
        en["agent.fact.noCalls"],
      ),
    );
    expect(container.querySelector(".arspend")).toBeNull();
    // Whether the panel is open is a fact about the block rather than about the
    // money, so the line the two share outlives the figure on it.
    expect(container.querySelector(".arlast > .archev")).not.toBeNull();
  });

  // The exact figure, not merely a figure: 120 minor units of the budget's
  // currency is $1.20, and the line the server could not price adds nothing
  // to it rather than turning the total into an unknown.
  it("sums the priced lines into the month's spend", async () => {
    stubAgentRailApi({ aiUsage: () => jsonResponse(PRICED_USAGE) });
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(container.querySelector(".arspend")?.textContent).toContain(
        "$1.20",
      ),
    );
    // And on the same line as the disclosure: one row of chrome, not a figure
    // with a chevron half a line below it.
    const foot = container.querySelector(".arlast");
    expect(foot?.querySelector(".arspend")).not.toBeNull();
    expect(foot?.querySelector(".archev")).not.toBeNull();
  });

  // The server serves the figure on `ai_diagnostics:read`, which the ops seat
  // holds and an edited role may hold; the cost is the administrator's figure
  // regardless, so a seat with the grant and without the role gets the runtime
  // row and no money — on the rail, in the panel head and in the meta row.
  it("shows the spend to no seat but an admin, grant or not", async () => {
    const user = userEvent.setup();
    const fetchMock = stubAgentRailApi({
      me: () => jsonResponse(meFixture({ roles: ["ops"], allow: OPERATOR })),
      aiUsage: () => jsonResponse(PRICED_USAGE),
    });
    const { container } = render(ROUTE);
    await openPanel(user, container);
    await waitFor(() =>
      expect(panel().querySelector(".armeta")?.textContent).toContain(
        en["agent.fact.noCalls"],
      ),
    );
    expect(container.querySelector(".arspend")).toBeNull();
    expect(panel().querySelector(".arpmoney")).toBeNull();
    expect(panel().querySelector(".armeta")?.textContent).not.toContain(
      en["agent.rail.spend"],
    );
    expect(
      container.querySelector(".artoggle")?.getAttribute("aria-label"),
    ).not.toContain(en["agent.rail.spend"]);
    // Withheld at the source, not only at the paint: a figure the seat may not
    // see is a figure the client never asks for.
    expect(
      fetchMock.mock.calls.some(([request]) =>
        new URL(request.url).pathname.endsWith("/ai/usage"),
      ),
    ).toBe(false);
  });

  // Only a company record serves a 360 read; every other screen must not ask
  // for one at all, not just decline to show it.
  it("never asks for the 360 read when the route is not a company record", async () => {
    const fetchMock = stubAgentRailApi();
    render(ROUTE);
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    const askedFor360 = fetchMock.mock.calls.some(([request]) =>
      new URL(request.url).pathname.endsWith("/360"),
    );
    expect(askedFor360).toBe(false);
  });

  // The screen's margins draw what this component publishes, and they cannot
  // work it out for themselves: the reads are all local here, so a margin that
  // re-derived the state would report on a second agent.
  it("publishes what the margins draw, and goes quiet when it unmounts", async () => {
    clearAgentEdge();
    stubAgentRailApi({
      approvals: () =>
        jsonResponse({ data: [APPROVAL("a-1")], page: emptyPage }),
    });
    const view = render(ROUTE);
    // The unanswered queue reaches a reader through this panel's own line, with
    // its count. It is not published to the margins: a contour standing for as
    // long as the queue does is a ring around the window on any installation
    // with work in it.
    await settlesOnLine(
      view.container,
      en["agent.line.waiting_one"].replace("{count}", "1"),
    );
    expect(currentAgentEdge()).toEqual(AGENT_EDGE_STILL);

    // Sign out mid-read and the login screen would otherwise inherit a lit
    // margin belonging to a session that has ended.
    view.unmount();
    expect(currentAgentEdge()).toEqual(AGENT_EDGE_STILL);
  });

  // The Core's own tone rule is the only place a state's colour is declared
  // (agentrail.css comment above `.arblock`), so a state added to the
  // vocabulary with no rule here would silently draw the accent default
  // instead of its own tone. Only "idle" is genuinely undocumented — it is
  // the section's resting state and carries no attribute rule of its own.
  it("carries a CSS tone rule for every state in the vocabulary but the resting default", () => {
    const cssPath = join(
      dirname(fileURLToPath(import.meta.url)),
      "agentrail.css",
    );
    const css = readFileSync(cssPath, "utf8");
    const documentedDefaults = new Set(["idle"]);
    for (const state of VOCABULARY) {
      if (documentedDefaults.has(state)) continue;
      expect(css).toContain(`data-core-state="${state}"`);
    }
  });
});
