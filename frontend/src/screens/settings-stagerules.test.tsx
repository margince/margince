/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { StageAutomationCard } from "./settings.stageautomation";
import { jsonResponse, PIPELINE_ADMIN, render } from "./settings.testkit";

// The controls that decide what a transition may do.
//
// Every test here turns on one distinction: ASKING IS NOT BEING ALLOWED.
// Turning a transition on records what an admin wants; the server re-checks
// the record in the transaction that would apply each move. A screen that said
// otherwise would have somebody believe deals were moving when they were not,
// or the reverse.

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
});

const PIPELINES = [{ id: "p1", name: "Sales", stages: [], version: 1 }];

function transition(
  from: string,
  to: string,
  fromName: string,
  toName: string,
) {
  return {
    pipeline_id: "p1",
    from_stage_id: from,
    to_stage_id: to,
    from_stage_name: fromName,
    to_stage_name: toName,
    reviewed: 240,
    proposed: 0,
    expired: 0,
    superseded: 0,
    accepted_clean: 236,
    accepted_edited: 2,
    rejected: 2,
    auto_applied: 0,
    unsafe: 1,
    observation_days: 34,
    clean_acceptance_rate: 0.98,
    edit_rate: 0.01,
    rejection_rate: 0.01,
    unsafe_rate: 0.004,
    evidence_kinds: [],
  };
}

const REPORT = {
  window_days: 30,
  data: [
    transition("s1", "s2", "Discovery", "Negotiation"),
    transition("s2", "s3", "Negotiation", "Contract"),
  ],
};

// A rule on the first transition; the second has none, which is the default
// state of every transition in the product.
const RULES = {
  data: [
    {
      id: "r1",
      pipeline_id: "p1",
      from_stage_id: "s1",
      to_stage_id: "s2",
      mode: "auto",
      clean_acceptance_threshold: 0.95,
      correction_reversal_threshold: 0.01,
      min_reviewed: 200,
      min_observation_days: 28,
      window_days: 30,
      undo_window_hours: 72,
      version: 3,
    },
  ],
};

type Recorded = { url: string; method: string; body: unknown };

function rulesStub(rules: unknown = RULES, grants = PIPELINE_ADMIN) {
  const calls: Recorded[] = [];
  const fetchStub = vi.fn(async (input: RequestInfo | URL) => {
    // openapi-fetch dispatches a Request object, so both the url and the
    // method ride it — read off a second argument they are always undefined,
    // and a test asserting "it did not write" would pass over any write.
    const request = input instanceof Request ? input : undefined;
    const url = String(request ? request.url : input);
    const method = request?.method ?? "GET";
    if (request && method !== "GET") {
      calls.push({ url, method, body: await request.clone().json() });
    }
    if (url.endsWith("/v1/me")) {
      return jsonResponse(meFixture({ roles: ["admin"], allow: grants }));
    }
    if (url.includes("/stage-automation/policies")) {
      return jsonResponse(method === "GET" ? rules : { ...RULES.data[0] });
    }
    if (url.includes("/stage-automation/report")) {
      return jsonResponse(REPORT);
    }
    if (url.includes("/pipelines")) {
      return jsonResponse({ data: PIPELINES, page: {} });
    }
    return jsonResponse({ data: [], page: {} });
  });
  return { fetchStub, calls };
}

describe("stage automation rules", () => {
  it("draws a switch for every transition, not only those with a rule", async () => {
    const { fetchStub } = rulesStub();
    vi.stubGlobal("fetch", fetchStub);
    render(<StageAutomationCard />);

    // Both transitions, though only one has a rule. A transition nobody has
    // decided about is the DEFAULT, and drawing only the decided ones would
    // hide every transition an admin has yet to think about.
    expect(
      await screen.findByRole("switch", { name: /Discovery → Negotiation/ }),
    ).toBeTruthy();
    expect(
      screen.getByRole("switch", { name: /Negotiation → Contract/ }),
    ).toBeTruthy();
  });

  it("shows a transition with a rule as on, and one without as off", async () => {
    const { fetchStub } = rulesStub();
    vi.stubGlobal("fetch", fetchStub);
    render(<StageAutomationCard />);

    const withRule = await screen.findByRole("switch", {
      name: /Discovery → Negotiation/,
    });
    const without = screen.getByRole("switch", {
      name: /Negotiation → Contract/,
    });
    expect(withRule.getAttribute("aria-checked")).toBe("true");
    expect(without.getAttribute("aria-checked")).toBe("false");
  });

  it("sends the version it read, so a save cannot overwrite a change it never saw", async () => {
    const { fetchStub, calls } = rulesStub();
    vi.stubGlobal("fetch", fetchStub);
    render(<StageAutomationCard />);

    const toggle = await screen.findByRole("switch", {
      name: /Discovery → Negotiation/,
    });
    await userEvent.click(toggle);

    await waitFor(() => expect(calls.length).toBeGreaterThan(0));
    const save = calls.find((call) => call.method === "PUT");
    expect(save).toBeTruthy();
    const body = save?.body as Record<string, unknown>;
    expect(body.mode).toBe("propose");
    expect(body.if_version).toBe(3);
    expect(body.from_stage_id).toBe("s1");
    expect(body.to_stage_id).toBe("s2");
  });

  it("sends no version for a transition that has no rule yet", async () => {
    const { fetchStub, calls } = rulesStub();
    vi.stubGlobal("fetch", fetchStub);
    render(<StageAutomationCard />);

    const toggle = await screen.findByRole("switch", {
      name: /Negotiation → Contract/,
    });
    await userEvent.click(toggle);

    await waitFor(() => expect(calls.length).toBeGreaterThan(0));
    const body = calls.find((call) => call.method === "PUT")?.body as Record<
      string,
      unknown
    >;
    // There was no row to have read, so a pin would be a claim about a version
    // that never existed — and the server answers 409 to one.
    expect(body.if_version).toBeUndefined();
    expect(body.mode).toBe("auto");
  });

  it("shows a suspended rule with the reason the product gave", async () => {
    const suspended = {
      data: [
        {
          ...RULES.data[0],
          suspended_at: "2026-09-03T10:00:00Z",
          suspended_reason:
            "1.2% of 240 reviewed moves on this transition were undone or corrected, above the 1.0% ceiling",
        },
      ],
    };
    const { fetchStub } = rulesStub(suspended);
    vi.stubGlobal("fetch", fetchStub);
    render(<StageAutomationCard />);

    // The reason itself, not a summary of it. It is the whole basis on which
    // somebody decides to start the transition again.
    expect(await screen.findByText(/above the 1.0% ceiling/)).toBeTruthy();
    expect(screen.getByRole("button", { name: /Start again/ })).toBeTruthy();
  });

  it("asks before starting a suspended transition again, and says the bar still applies", async () => {
    const suspended = {
      data: [
        {
          ...RULES.data[0],
          suspended_at: "2026-09-03T10:00:00Z",
          suspended_reason: "a move reached a record outside its own workspace",
        },
      ],
    };
    const { fetchStub, calls } = rulesStub(suspended);
    vi.stubGlobal("fetch", fetchStub);
    render(<StageAutomationCard />);

    await userEvent.click(
      await screen.findByRole("button", { name: /Start again/ }),
    );

    // Nothing sent yet: the confirmation is the point, and a resume that fired
    // on the first click would lift a safety stop by mis-click.
    expect(calls.filter((call) => call.url.includes("/resume"))).toHaveLength(
      0,
    );
    // The dialog says what starting again does NOT do.
    expect(await screen.findByText(/does not skip the bar/i)).toBeTruthy();
  });

  it("does not offer a way back on a rule the product has not stopped", async () => {
    const { fetchStub } = rulesStub();
    vi.stubGlobal("fetch", fetchStub);
    render(<StageAutomationCard />);

    await screen.findByRole("switch", { name: /Discovery → Negotiation/ });
    // A running rule has nothing to resume, and a control offering it would
    // ask for a decision about a state the transition is not in.
    expect(screen.queryByRole("button", { name: /Start again/ })).toBeNull();
  });

  it("refuses the controls to a reader who may see the report and not change it", async () => {
    // The report opens on `pipeline` READ; every control here needs update. A
    // viewer holding only the read must see the evidence with the controls
    // REFUSED, not live ones that 403 on the first click.
    const { fetchStub, calls } = rulesStub(
      {
        data: [
          {
            ...RULES.data[0],
            suspended_at: "2026-09-03T10:00:00Z",
            suspended_reason:
              "a move reached a record outside its own workspace",
          },
        ],
      },
      { pipeline: ["read"] },
    );
    vi.stubGlobal("fetch", fetchStub);
    render(<StageAutomationCard />);

    const toggle = await screen.findByRole("switch", {
      name: /Discovery → Negotiation/,
    });
    // The real `disabled` property. The design system reserves aria-disabled
    // for a control that is mid-write and must keep focus; one nobody may
    // touch is disabled outright.
    expect((toggle as HTMLButtonElement).disabled).toBe(true);
    // The way back is absent rather than refused: resuming is a whole act, and
    // a disabled control for one is a door that was never theirs to open.
    expect(screen.queryByRole("button", { name: /Start again/ })).toBeNull();
    // And the reason is still readable — this reader came to consult it.
    expect(
      screen.getByText(/a move reached a record outside its own workspace/),
    ).toBeTruthy();

    await userEvent.click(toggle);
    expect(calls.filter((call) => call.method === "PUT")).toHaveLength(0);
  });

  it("says nothing about thresholds when the rule is counted over a different window", async () => {
    // The report is fetched over 30 days; this rule counts its own rates over
    // 7. Judging one against the other would say a transition had earned a bar
    // it was never measured against.
    const otherWindow = {
      data: [{ ...RULES.data[0], window_days: 7, min_reviewed: 400 }],
    };
    const { fetchStub } = rulesStub(otherWindow);
    vi.stubGlobal("fetch", fetchStub);
    render(<StageAutomationCard />);

    await screen.findByRole("switch", { name: /Discovery → Negotiation/ });
    // 240 reviewed is short of 400, so a naive comparison would draw the line.
    expect(screen.queryByText(/Not earned yet/)).toBeNull();
  });

  it("says which threshold a transition is short of", async () => {
    const shortOfVolume = {
      data: [{ ...RULES.data[0], min_reviewed: 400 }],
    };
    const { fetchStub } = rulesStub(shortOfVolume);
    vi.stubGlobal("fetch", fetchStub);
    render(<StageAutomationCard />);

    // A switch that says ON while the server keeps proposing is this page's
    // worst failure, so the line names the bar rather than leaving the admin
    // to wonder why cards keep arriving.
    expect(await screen.findByText(/240\/400/)).toBeTruthy();
  });

  it("says how long an automatic move can be taken back", async () => {
    const { fetchStub } = rulesStub();
    vi.stubGlobal("fetch", fetchStub);
    render(<StageAutomationCard />);

    // Drawn only where it means something: a transition that proposes has no
    // automatic move to undo.
    expect(await screen.findByText(/Undo for 72 h/)).toBeTruthy();
  });
});
