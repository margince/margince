/** @vitest-environment jsdom */

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import {
  QueryClient,
  QueryClientProvider,
  useMutation,
} from "@tanstack/react-query";
import {
  act,
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import type { ReactNode, RefObject } from "react";
import { useEffect } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { clearAgentEdge, currentAgentEdge } from "./agent-edge-signal";
import { AgentRail } from "./agentrail";
import { LABELS, REVIEW_ONLY, TASK_SAID, VOCABULARY } from "./agentrail-copy";
import { type GrantSpec, meFixture } from "./mefixture";
import type { Route } from "./router";
import { stubPhoneViewport } from "./testing/shellharness";

// The agent section at the foot of the workspace rail reads every one of its
// facts off the wire (approvals, connectors, dedupe, AI posture, licence
// entitlement, the served model, the month's spend, the account's own
// suggestions) — nothing left standing in for a read that has not answered.
// Every case here proves the section draws exactly what the API said, and
// never a number nobody computed.
//
// The state derivation is load-bearing enough to earn cases of its own: red is
// for the tool not being reachable at all (no model bound, or a source it
// cannot get to), amber is for a licence that is not clean, and a transient
// tool failure colours nothing — that precision is the whole point of moving
// off the eight-state vocabulary.

type Connector = components["schemas"]["CaptureConnection"];
type Candidate = components["schemas"]["DedupeCandidate"];
type Approval = components["schemas"]["Approval"];
type AiCallSummary = components["schemas"]["AiCallSummary"];
type AssistantProfile = components["schemas"]["AssistantProfile"];
type LicenseEntitlement = components["schemas"]["LicenseEntitlement"];

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const emptyPage = { has_more: false, next_cursor: null };

const CONNECTED: Connector = {
  id: "018f3a1b-0000-7000-8000-0000000000c1",
  provider: "gmail",
  status: "connected",
  scopes: ["https://www.googleapis.com/auth/gmail.readonly"],
};

const CANDIDATE = (id: string): Candidate => ({
  id,
  entity_type: "organization",
  left_id: "o-1",
  right_id: "o-2",
  confidence: 0.91,
  evidence: [],
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
};

const OPERATOR: GrantSpec = { automation: ["update"], license: ["read"] };

const PROFILE = (state: AssistantProfile["state"]): AssistantProfile => ({
  name: "Margince",
  kind: "ai",
  state,
  inference_mode:
    state === "development"
      ? "development"
      : state === "unconfigured"
        ? "none"
        : "cloud",
  providers: [],
});

const LICENSE = (state: LicenseEntitlement["state"]): LicenseEntitlement => ({
  state,
  seats_used: 1,
  over_limit: false,
  checked_at: "2026-08-01T09:00:00Z",
});

const EMPTY_USAGE = {
  days: [],
  budget: { monthly_tokens: 0, spent_tokens: 0, band: "normal" as const },
};

const ORG_360_VIEW = {
  as_of: "2026-08-01T09:00:00Z",
  organization: {
    id: "o-1",
    display_name: "Brandt Automotive GmbH",
    captured_by: "human:u1",
    source: "manual",
    version: 1,
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-06-01T08:00:00Z",
  },
  sections_omitted: [],
  people: { data: [], page: emptyPage },
  deals: {
    data: [],
    page: emptyPage,
    won_lifetime: { amount_minor: 0, currency: "EUR" },
    lost_count: 0,
  },
  activities: { data: [], page: emptyPage },
  next_steps: { data: [], page: emptyPage },
  pending_approvals: { data: [], page: emptyPage },
  tags: [],
  list_memberships: [],
  since_last_visit: {
    baseline_at: "2026-05-30T09:00:00Z",
    new_activities: 0,
    deal_stage_moves: 0,
    pending_proposals: 0,
  },
};

type FetchRoutes = {
  connectors?: () => Response | Promise<Response>;
  dedupe?: () => Response | Promise<Response>;
  approvals?: () => Response | Promise<Response>;
  aiCalls?: () => Response | Promise<Response>;
  aiUsage?: () => Response | Promise<Response>;
  me?: () => Response | Promise<Response>;
  profile?: () => Response | Promise<Response>;
  license?: () => Response | Promise<Response>;
  agentActivity?: () => Response | Promise<Response>;
};

// Routes every hook the section depends on by pathname, the way
// company360.test.tsx does for its own composite read. The healthy default
// answers every source as connected, every queue empty, the seat allowed to
// read the runtime and the licence, the AI posture as configured and the
// licence as valid — each test overrides only the one route its case is
// about. Returns the stubbed fetch mock itself, so a case that needs to prove
// a request was never MADE (the 360 read off a company page) has something to
// inspect.
function stubAgentRailApi(routes: FetchRoutes = {}) {
  const fetchMock = vi.fn(async (request: Request) => {
    const pathname = new URL(request.url).pathname;
    if (pathname.endsWith("/connectors")) {
      return routes.connectors
        ? routes.connectors()
        : jsonResponse({ data: [CONNECTED] });
    }
    if (pathname.endsWith("/dedupe/candidates")) {
      return routes.dedupe
        ? routes.dedupe()
        : jsonResponse({ data: [], page: emptyPage });
    }
    if (pathname.endsWith("/approvals")) {
      return routes.approvals
        ? routes.approvals()
        : jsonResponse({ data: [], page: emptyPage });
    }
    if (pathname.endsWith("/ai/calls")) {
      return routes.aiCalls
        ? routes.aiCalls()
        : jsonResponse({
            data: [],
            page: emptyPage,
            payload_capture_enabled: false,
            tasks: [],
          });
    }
    if (pathname.endsWith("/ai/usage")) {
      return routes.aiUsage ? routes.aiUsage() : jsonResponse(EMPTY_USAGE);
    }
    if (pathname.endsWith("/me/ai-activity")) {
      return routes.agentActivity
        ? routes.agentActivity()
        : jsonResponse({ running: [], recent: [] });
    }
    if (pathname.endsWith("/me")) {
      return routes.me
        ? routes.me()
        : jsonResponse(meFixture({ allow: OPERATOR }));
    }
    if (pathname.endsWith("/assistant/profile")) {
      return routes.profile
        ? routes.profile()
        : jsonResponse(PROFILE("configured"));
    }
    if (pathname.endsWith("/installation/license")) {
      return routes.license ? routes.license() : jsonResponse(LICENSE("valid"));
    }
    if (pathname.endsWith("/360")) {
      return jsonResponse({ state: "ready", view: ORG_360_VIEW });
    }
    return jsonResponse({ data: [], page: emptyPage });
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

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

function render(
  route: Route,
  options: Readonly<{
    client?: QueryClient;
    children?: ReactNode;
    bar?: RefObject<HTMLElement | null>;
  }> = {},
) {
  const client =
    options.client ??
    new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        {options.children}
        <AgentRail route={route} bar={options.bar} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const ROUTE: Route = { screen: "deals" };

const block = (container: HTMLElement) => {
  const el = container.querySelector(".arblock");
  if (!el) throw new Error("no .arblock in the rendered tree");
  return el;
};

const openPanel = async (user: UserEvent, container: HTMLElement) => {
  const trigger = container.querySelector(".arhit");
  if (!trigger) throw new Error("no .arhit trigger in the rendered tree");
  await user.click(trigger);
};

// The panel is portalled to document.body once open, so its content is never
// found inside the render container.
const panel = () => {
  const el = document.querySelector(".arloose");
  if (!el) throw new Error("no .arloose panel on the document");
  return el;
};

/**
 * The state's own resting line, once the ticker has finished naming the
 * mount-time reads.
 *
 * Every query the section holds starts a `useAgentTicker` line the moment it
 * fetches (agentrail-ticker.ts), and that line outlives its own request by
 * `LINGER_MS` (1400ms) so a fast read is still readable. A case that asserts
 * the state's own line has to wait past that window, or it is really
 * asserting about the ticker's last word instead.
 */
// Named so it does NOT begin with `waitFor`: scripts/test-budget.ts treats that
// prefix as a library waiter and stops being able to fold the budget inside a
// local helper that wears it.
async function settlesOnLine(container: HTMLElement, expected: string) {
  await waitFor(
    () =>
      expect(container.querySelector(".arline")?.textContent).toBe(expected),
    { timeout: 3000 },
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
  // The frame case spies on Element.prototype.getBoundingClientRect, which every
  // later case in this file shares. Restored here rather than there: a spy left
  // on a prototype is the kind of leak whose failure names a case that is fine.
  vi.restoreAllMocks();
  vi.useRealTimers();
});

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
    await settlesOnLine(container, LABELS.allClear);
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
    await settlesOnLine(container, LABELS.noModel);
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
      expect(runtime).toContain("offline development path");
    });
  });

  // An installation that never had a licence is NOT a fault: that is the state
  // every demo and every fresh dev stack is in, and an orb that is amber for all
  // of them has stopped saying anything. Asked-and-refused is the fault, and the
  // case below still carries it. Issue 2679 carries where the absent licence
  // should be surfaced instead, which is not nowhere.
  it("rests rather than warning when the installation has no licence", async () => {
    // The licence answer is HELD and then released, and that is the whole
    // difference between this test and one that proves nothing. The posture hook
    // reads undefined until the query lands, and derive() calls that idle -- so
    // asserting "not warning" against a live stub can pass before the absent
    // licence has been seen at all, which is a green test for an assertion never
    // made. Holding the response makes the first assertion about the state
    // BEFORE the answer, and the second about the state after it.
    let release: () => void = () => {};
    const answered = new Promise<void>((resolve) => {
      release = resolve;
    });
    let delivered = false;
    stubAgentRailApi({
      license: async () => {
        await answered;
        delivered = true;
        return jsonResponse(LICENSE("absent"));
      },
    });
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("idle"),
    );

    release();
    await waitFor(() => expect(delivered).toBe(true));
    // The absent licence has now been answered and read, and the section still
    // rests. That the answer REACHES derive() at all is proven by the refused
    // case below, which is the same construction and does turn amber: without
    // that pair, an assertion of "still idle" could not tell a licence that was
    // seen and shrugged off from one that never arrived.
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("idle"),
    );
  });

  it("goes to warning when the licence is refused", async () => {
    // Held and released like the case above, and this is the half that makes the
    // pair mean something: the same pipeline, the same timing, and this one DOES
    // reach warning. So "still idle" up there is a licence that was read and
    // deliberately not escalated, rather than one that never landed.
    let release: () => void = () => {};
    const answered = new Promise<void>((resolve) => {
      release = resolve;
    });
    stubAgentRailApi({
      license: async () => {
        await answered;
        return jsonResponse(LICENSE("rejected"));
      },
    });
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("idle"),
    );

    release();
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("warning"),
    );
    await settlesOnLine(container, "License refused");
  });

  // A seat without `license:read` gets `undefined`, not a fault: a read this
  // seat may not make is none of its business, and must not turn the orb
  // amber on every screen it opens.
  it("gives neither a warning nor a licence row when the seat may not read the licence", async () => {
    const user = userEvent.setup();
    stubAgentRailApi({
      me: () => jsonResponse(meFixture({ allow: { automation: ["update"] } })),
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
  // not colour the orb: a corner that flashed red on a flaky connection is
  // exactly the failure mode this derivation was rebuilt to avoid.
  it("does not colour the orb for a transient tool failure", async () => {
    stubAgentRailApi({
      dedupe: () =>
        jsonResponse(
          { code: "internal_error", title: "internal error", status: 500 },
          500,
        ),
    });
    const { container } = render(ROUTE);
    // The LINE has to settle before the state is worth asserting: `.arline` is
    // in the markup from the first render, so waiting for the element alone
    // would let this pass before the 500 ever reached React Query, which is the
    // one moment the assertion is supposed to be about.
    await settlesOnLine(container, LABELS.allClear);
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
    await settlesOnLine(container, `3 ${LABELS.waiting}`);
    expect(block(container).querySelector(".arbadge")).toBeNull();
  });

  // The duplicate queue is not the agent's work. It is a queue the product
  // keeps, repaired on the screen that owns it, and it reached this surface in
  // three places at once: a turn in the resting rotation, a tile in the panel,
  // and a second telling of a number the worklist already carries. None of them
  // is here now, and the state was never one of them because a queue is not a
  // fault.
  it("says nothing about the duplicate queue, in the line, the state or the panel", async () => {
    const user = userEvent.setup();
    stubAgentRailApi({
      dedupe: () =>
        jsonResponse({ data: [CANDIDATE("d-1"), CANDIDATE("d-2")] }),
    });
    const { container } = render(ROUTE);
    await settlesOnLine(container, LABELS.allClear);
    expect(block(container).getAttribute("data-core-state")).toBe("idle");

    await openPanel(user, container);
    expect(
      screen.queryByRole("link", { name: /Duplicate pairs open/ }),
    ).toBeNull();
    expect(panel().textContent).not.toContain("Duplicate");
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
      screen.queryByRole("link", { name: new RegExp(`^${LABELS.approvals}`) }),
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
    expect(panel().textContent).not.toContain(LABELS.acrossWorkspace);
    expect(
      screen.queryByRole("link", { name: new RegExp(`^${LABELS.approvals}`) }),
    ).toBeNull();
  });

  it("says all-clear once the read answers zero", async () => {
    const user = userEvent.setup();
    stubAgentRailApi();
    const { container } = render(ROUTE);
    await openPanel(user, container);
    // Scoped to the workspace section on purpose: the resting LINE in the
    // header reads all-clear too, so a whole-panel match would still pass with
    // this paragraph gone and prove nothing about the answered zero.
    await waitFor(() =>
      expect(
        [...panel().querySelectorAll(".arsect")]
          .find((section) =>
            section.textContent?.includes(LABELS.acrossWorkspace),
          )
          ?.querySelector(".arnone")?.textContent,
      ).toBe(LABELS.allClear),
    );
    // A real zero is an answer, not a tile: "0 Decisions waiting" is a number
    // nobody has to act on dressed as one somebody does.
    expect(
      screen.queryByRole("link", { name: new RegExp(`^${LABELS.approvals}`) }),
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
          name: new RegExp(`^${LABELS.approvals}`),
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
    const trigger = container.querySelector(".arhit");
    expect(trigger?.getAttribute("aria-expanded")).toBe("false");

    await openPanel(user, container);
    expect(document.querySelector(".arloose")).not.toBeNull();
    expect(
      container.querySelector(".arhit")?.getAttribute("aria-expanded"),
    ).toBe("true");
  });

  // The state switcher is review-only scaffolding, and it is ON by default
  // while the surface is under design (app/ui-preview.ts). What has to stay
  // true is that an installation can take it away, which is the posture this
  // asserts: the switch turned off leaves no way to put the section into a
  // state no read can reach.
  it("carries no state switcher when the preview switch is turned off", async () => {
    vi.stubEnv("VITE_UI_PREVIEW_TASKBAR", "0");
    const user = userEvent.setup();
    stubAgentRailApi();
    const { container } = render(ROUTE);
    await openPanel(user, container);
    expect(panel().querySelector(".archips")).toBeNull();
  });

  it("carries the state switcher by default, so a reviewer finds it without a flag", async () => {
    const user = userEvent.setup();
    stubAgentRailApi();
    const { container } = render(ROUTE);
    await openPanel(user, container);
    expect(panel().querySelector(".archips")).toBeTruthy();
    expect(screen.getByRole("button", { name: LABELS.runPlay })).toBeTruthy();
  });

  it("lets a review-only chip override the derived state with its invented line, once the preview switch is on", async () => {
    vi.stubEnv("VITE_UI_PREVIEW_TASKBAR", "1");
    const user = userEvent.setup();
    stubAgentRailApi();
    const { container } = render(ROUTE);
    await openPanel(user, container);

    await user.click(screen.getByRole("button", { name: "working" }));
    expect(block(container).getAttribute("data-core-state")).toBe("working");
    await settlesOnLine(container, REVIEW_ONLY.working ?? "");
  });

  it("renders nothing on the full Ask surface", () => {
    stubAgentRailApi();
    const { container } = render({ screen: "ai" });
    expect(container.querySelector(".arblock")).toBeNull();
  });

  it("says the runtime row is not readable on a seat without automation:update", async () => {
    const user = userEvent.setup();
    stubAgentRailApi({
      me: () => jsonResponse(meFixture({ allow: { license: ["read"] } })),
    });
    const { container } = render(ROUTE);
    await openPanel(user, container);
    await waitFor(() =>
      expect(panel().querySelector(".armeta")?.textContent).toContain(
        LABELS.unreadable,
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
        LABELS.noCallsYet,
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
          tasks: [AI_CALL.task],
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
        LABELS.noCallsYet,
      ),
    );
    expect(container.querySelector(".arspend")).toBeNull();
  });

  it("sums the priced lines into the month's spend", async () => {
    stubAgentRailApi({
      aiUsage: () =>
        jsonResponse({
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
        }),
    });
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(container.querySelector(".arspend")?.textContent).toBeTruthy(),
    );
  });

  // The wire carries the invocation-site token (`capture_classify`); the
  // recap owes the reader the plain-language line, never the token itself —
  // a recap that leaked the token would tell a salesperson something ran five
  // times and nothing about what.
  it("recaps a call in plain words, not the raw task token", async () => {
    const user = userEvent.setup();
    stubAgentRailApi({
      aiCalls: () =>
        jsonResponse({
          data: [AI_CALL],
          page: emptyPage,
          payload_capture_enabled: false,
          tasks: [AI_CALL.task],
        }),
    });
    const { container } = render(ROUTE);
    await openPanel(user, container);
    await waitFor(() =>
      expect(screen.getByText(TASK_SAID.capture_classify)).toBeTruthy(),
    );
    expect(panel().textContent).not.toContain("capture_classify");
    expect(
      screen.getByRole("link", { name: LABELS.fullLog }).getAttribute("href"),
    ).toBe("#/settings/admin/ai");
  });

  // A task named `constructor` is a plain string off the wire, but a bare
  // lookup into an object answers it from `Object.prototype` with a function
  // React then tries to render — `saidFor` guards with `Object.hasOwn`
  // precisely so the recap still shows the humanised token.
  it("shows the humanised token, not a function, for a task named constructor", async () => {
    const user = userEvent.setup();
    stubAgentRailApi({
      aiCalls: () =>
        jsonResponse({
          data: [{ ...AI_CALL, task: "constructor" }],
          page: emptyPage,
          payload_capture_enabled: false,
          tasks: ["constructor"],
        }),
    });
    const { container } = render(ROUTE);
    await openPanel(user, container);
    await waitFor(() => expect(screen.getByText("constructor")).toBeTruthy());
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
  // work it out for themselves: the run, the switcher and the reads are all local
  // here, so a margin that re-derived the state would report on a second agent.
  it("publishes what the margins draw, and goes quiet when it unmounts", async () => {
    clearAgentEdge();
    stubAgentRailApi({
      approvals: () =>
        jsonResponse({ data: [APPROVAL("a-1")], page: emptyPage }),
    });
    const view = render(ROUTE);
    await settlesOnLine(view.container, `1 ${LABELS.waiting}`);
    expect(currentAgentEdge().waiting).toBe(true);

    // Sign out mid-read and the login screen would otherwise inherit a lit
    // margin belonging to a session that has ended.
    view.unmount();
    expect(currentAgentEdge()).toEqual({ reading: false, waiting: false });
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

  // Work THIS TAB DID NOT START. Everything above moves the Core because a
  // query in this browser fetched; these cases cover the overnight runner,
  // reported through /me/ai-activity, plus the two invariants that must
  // hold alongside it: operator vocabulary stays out of the reader's line, and
  // the surface never invents a sentence for a kind it has no copy for.

  /** One run as the WIRE spells it — a JSON body, not a typed literal, because
   *  a kind that is off the contract is exactly what one of these cases sends. */
  const RUN = (over: Readonly<Record<string, unknown>> = {}) => ({
    id: "019f7e65-fbf7-7114-b114-40af4af63a01",
    kind: "morning_brief",
    state: "running",
    started_at: "2026-08-21T05:00:00Z",
    ...over,
  });

  const withRuns = (...running: readonly unknown[]) =>
    stubAgentRailApi({
      agentActivity: () => jsonResponse({ running, recent: [] }),
    });

  /** A run that has SETTLED. The read puts these in `recent` and never in
   *  `running` (compose/agentactivity `recentSQL` vs `runningSQL`), and
   *  `degrade_reason` and `summary` are written alongside the terminal status —
   *  so this is the only shape those two fields ever arrive in. */
  const withSettled = (...recent: readonly unknown[]) =>
    stubAgentRailApi({
      agentActivity: () => jsonResponse({ running: [], recent }),
    });

  const BRIEF_RUNNING = "I'm putting your morning brief together.";

  const runLines = () =>
    [...panel().querySelectorAll(".arrunline")].map((el) => el.textContent);

  // The work a person ASKS for, which is the case the rail was silent on until
  // the router could say `running`. Asserted at RENDER and not against the map:
  // the copy existing is not the claim — the claim is that a live summarize
  // reaches the reader's own line, in their own words, through the same feed
  // the overnight run uses.
  it("narrates a summary the reader asked for while it is still being written", async () => {
    withRuns(RUN({ kind: "summarize" }));
    const { container } = render(ROUTE);
    await settlesOnLine(
      container,
      "I'm pulling together what I know about this company.",
    );
  });

  it("moves the Core to working when a server run is live and this tab is idle", async () => {
    withRuns(RUN());
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("working"),
    );
  });

  it("names the run in the rail line", async () => {
    withRuns(RUN());
    const { container } = render(ROUTE);
    await settlesOnLine(container, BRIEF_RUNNING);
  });

  // Two subjects, two slots. They used to share one, so the agent's sentence
  // disappeared for as long as anything was loading and a reader could not tell
  // which of the two was talking. Both are true at the same moment here: an
  // overnight brief is running, and this tab is fetching its own sources.
  it("keeps the agent's line and the tool's narration in separate slots", async () => {
    vi.useFakeTimers();
    stubAgentRailApi({
      agentActivity: () => jsonResponse({ running: [RUN()], recent: [] }),
    });
    const { container } = render(ROUTE);
    // Inside the ticker's LINGER_MS, so the mount-time reads it named are still
    // standing while the agent's own line has answered. That overlap IS the
    // case: the two used to take turns in one slot, and the whole point of the
    // change is that this moment shows both.
    await act(() => vi.advanceTimersByTimeAsync(300));
    expect(container.querySelector(".arline")?.textContent).toBe(BRIEF_RUNNING);
    // WHICH of the mount reads is newest is a race between stubs and no part of
    // the claim. That the tool has a line of its own, saying something other
    // than the agent's, is the whole claim.
    const tool = container.querySelector(".artool")?.textContent;
    expect(tool).toBeTruthy();
    expect(tool).not.toBe(BRIEF_RUNNING);
  });

  // The colour and the sentence are always about the SAME thing. A workspace in
  // grace keeps running its agent, so amber-for-the-licence and a live run are
  // true at once — and the licence outranks the run, which means the run's
  // sentence must not caption it. Captioning an amber orb "I'm putting your
  // morning brief together" tells a reader the brief is the fault.
  it("never captions a state with a run that did not cause it", async () => {
    stubAgentRailApi({
      license: () =>
        jsonResponse({
          state: "rejected",
          seats_used: 1,
          over_limit: false,
          checked_at: "2026-08-01T09:00:00Z",
        }),
      agentActivity: () => jsonResponse({ running: [RUN()], recent: [] }),
    });
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("warning"),
    );
    expect(container.querySelector(".arline")?.textContent).not.toBe(
      BRIEF_RUNNING,
    );
  });

  // Opening the panel acknowledges EVERY fault the read reported, at once.
  // Clearing only the fault the orb happened to show would leave the colour up
  // after the reader had already turned to the report.
  it("clears every fault the panel delivered, not one per open", async () => {
    window.localStorage.removeItem("margince.agent.faults-seen");
    withSettled(
      RUN({ id: "019f7e65-fbf7-7114-b114-40af4af63c01", state: "failed" }),
      RUN({ id: "019f7e65-fbf7-7114-b114-40af4af63c02", state: "degraded" }),
    );
    const user = userEvent.setup();
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("error"),
    );
    await openPanel(user, container);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("idle"),
    );
  });

  // The other half of the live vocabulary, and the half that had no producer at
  // all until the lane came off the KIND of work: evidence arriving is `ingest`,
  // reasoning over evidence already held is `working` (ai-activity-orb.ts).
  it("moves the Core to ingest when the live run is evidence arriving", async () => {
    withRuns(RUN({ kind: "document_extract" }));
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("ingest"),
    );
  });

  // A run past the lease its own source declared. The server derives it, so a
  // worker that died without saying so cannot go on being displayed as busy —
  // and amber is right for it, because the work may yet land and there is
  // nothing for the reader to do but know.
  it("moves the Core to warning for a run past its own lease", async () => {
    withRuns(RUN({ state: "stalled" }));
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("warning"),
    );
  });

  // The overnight case, which is the whole reason a fault is acknowledged rather
  // than decayed: a run fails at four in the morning and the person it ran for
  // is asleep, so whatever the orb does at 04:12 is seen by nobody. It holds
  // until the panel has actually been opened.
  it("holds a failed run on the Core until the panel has been opened", async () => {
    // Acknowledgement is remembered per browser, so the case owns its own id and
    // its own storage: sharing either would make this test's verdict depend on
    // which of its neighbours ran first.
    window.localStorage.removeItem("margince.agent.faults-seen");
    withSettled(
      RUN({ id: "019f7e65-fbf7-7114-b114-40af4af63b02", state: "failed" }),
    );
    const user = userEvent.setup();
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("error"),
    );
    await openPanel(user, container);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("idle"),
    );
  });

  // Above the counts, because a run happening right now outranks a queue that
  // has been waiting since yesterday.
  it("lists running work in the panel under its own heading", async () => {
    withRuns(RUN());
    const user = userEvent.setup();
    const { container } = render(ROUTE);
    await openPanel(user, container);
    await waitFor(() => expect(runLines()).toEqual([BRIEF_RUNNING]));
    const headings = [...panel().querySelectorAll(".arsect h4")].map(
      (el) => el.textContent,
    );
    expect(headings[0]).toBe("Running now");
    expect(headings).toContain(LABELS.acrossWorkspace);
  });

  // `degrade_reason` is server-authored operator vocabulary and untranslated.
  // The reader's surfaces carry the localized line and nothing else, so the
  // raw token reaches no surface at all.
  it("keeps the degrade reason out of the line and out of the panel", async () => {
    const reason = "brief_partial: crm_read_timeout";
    const stopped = "I got partway through your morning brief and stopped.";
    withSettled(RUN({ state: "degraded", degrade_reason: reason }));
    const user = userEvent.setup();
    const { container } = render(ROUTE);
    await settlesOnLine(container, stopped);
    expect(container.querySelector(".arline")?.textContent).not.toContain(
      reason,
    );
    await openPanel(user, container);
    expect(panel().textContent).not.toContain(reason);
  });

  // A kind the copy map has never heard of produces NOTHING: no fallback
  // sentence, no raw message key, no de-underscored token. The run is still
  // real, so the orb still moves — only the words are missing.
  it("renders no line for a kind it has no copy for", async () => {
    withRuns(RUN({ kind: "telepathic_prospecting" }));
    const user = userEvent.setup();
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("working"),
    );
    await openPanel(user, container);
    expect(runLines()).toEqual([]);
    expect(panel().textContent).not.toContain("Running now");
    expect(panel().textContent).not.toContain("agent.activity");
    expect(panel().textContent).not.toContain("telepathic_prospecting");
  });

  // A settled run reaches the reader through the RESTING ROTATION on the card
  // and nowhere else: the panel lists live work only, so a day of finished
  // runs draws no list there. `recent` is bounded to today, so the rotation
  // never pins a "ready" that would still be announcing this morning at six
  // in the evening.
  it("reads a run that finished today in the resting line, not as a panel list", async () => {
    withSettled(RUN({ state: "done" }));
    const user = userEvent.setup();
    const { container } = render(ROUTE);
    // Nothing is live, so the orb rests — and the line is the settled run,
    // which is the only true thing this installation has to say.
    await settlesOnLine(container, "Your morning brief is ready.");
    expect(block(container).getAttribute("data-core-state")).toBe("idle");
    await openPanel(user, container);
    expect(runLines()).toEqual([]);
    expect(panel().textContent).not.toContain("Finished today");
    expect(panel().textContent).not.toContain("Running now");
  });
});
