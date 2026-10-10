/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { StageAutomationCard } from "./settings.stageautomation";
import { jsonResponse, PIPELINE_ADMIN, render } from "./settings.testkit";

// What a transition has earned, as a reader sees it.
//
// The screen decides nothing — it is the evidence somebody weighs before
// trusting a transition to move deals on its own. So what is under test is
// whether the numbers arrive intact and whether the two cases that look alike
// on screen stay told apart: a transition contacts reject, and one nobody has
// answered.

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
});

const PIPELINES = [{ id: "p1", name: "Sales", stages: [], version: 1 }];

// One transition with a real record, and one proposed but never answered.
const REPORT = {
  window_days: 30,
  data: [
    {
      pipeline_id: "p1",
      from_stage_id: "s1",
      to_stage_id: "s2",
      from_stage_name: "Discovery",
      to_stage_name: "Negotiation",
      reviewed: 20,
      proposed: 3,
      expired: 2,
      superseded: 0,
      accepted_clean: 17,
      accepted_edited: 2,
      rejected: 1,
      auto_applied: 0,
      unsafe: 1,
      observation_days: 34,
      clean_acceptance_rate: 0.85,
      edit_rate: 0.1,
      rejection_rate: 0.05,
      unsafe_rate: 0.05,
      evidence_kinds: [
        {
          kind: "document_signed",
          reviewed: 12,
          accepted_clean: 12,
          unsafe: 0,
        },
      ],
    },
    {
      pipeline_id: "p1",
      from_stage_id: "s2",
      to_stage_id: "s3",
      from_stage_name: "Negotiation",
      to_stage_name: "Contract",
      reviewed: 0,
      proposed: 4,
      expired: 0,
      superseded: 0,
      accepted_clean: 0,
      accepted_edited: 0,
      rejected: 0,
      auto_applied: 0,
      unsafe: 0,
      observation_days: 0,
      clean_acceptance_rate: 0,
      edit_rate: 0,
      rejection_rate: 0,
      unsafe_rate: 0,
      evidence_kinds: [],
    },
  ],
};

function reportStub(report: unknown = REPORT, pipelines: unknown = PIPELINES) {
  return vi.fn(async (input: RequestInfo | URL) => {
    // openapi-fetch dispatches a Request, so the url rides the object rather
    // than a string argument.
    const url = String(input instanceof Request ? input.url : input);
    if (url.endsWith("/v1/me")) {
      return jsonResponse(
        meFixture({ roles: ["admin"], allow: PIPELINE_ADMIN }),
      );
    }
    if (url.includes("/stage-automation/report")) {
      return jsonResponse(report);
    }
    if (url.includes("/pipelines")) {
      return jsonResponse({ data: pipelines, page: {} });
    }
    return jsonResponse({ data: [], page: {} });
  });
}

describe("stage automation report", () => {
  it("shows reviewed volume, observation days, and every rate per transition", async () => {
    vi.stubGlobal("fetch", reportStub());
    render(<StageAutomationCard />);

    const row = (await screen.findByText(/Discovery → Negotiation/)).closest(
      "tr",
    );
    expect(row).toBeTruthy();
    const cells = Array.from(row?.querySelectorAll("td") ?? []).map(
      (cell) => cell.textContent,
    );
    // The volume the rates are computed over, drawn beside them rather than
    // behind a tooltip: 85% over twenty cards and 85% over four are different
    // claims, and only the count says which this is.
    expect(cells).toContain("20");
    expect(cells).toContain("85%");
    expect(cells).toContain("10%");
    expect(cells).toContain("5%");
  });

  it("keeps the span watched one press away, beside the counts no rate shows", async () => {
    vi.stubGlobal("fetch", reportStub());
    const user = userEvent.setup();
    render(<StageAutomationCard />);

    await user.click(
      await screen.findByRole("button", {
        name: "Details for Discovery → Negotiation",
      }),
    );
    // A fine rate earned in one afternoon is not a record, and this is the
    // number that says so.
    const facts = Array.from(
      document.querySelectorAll("tr.datatable-detail .factlist-row"),
    ).map((row) => row.textContent);
    expect(facts.slice(0, 3)).toEqual([
      "Still open3",
      "Expired2",
      "Days observed34",
    ]);
  });

  it("sets every figure against the end of its column, heading included", async () => {
    vi.stubGlobal("fetch", reportStub());
    render(<StageAutomationCard />);

    const table = await screen.findByRole("table", {
      name: "Stage automation",
    });
    const headings = within(table)
      .getAllByRole("columnheader")
      .filter((heading) => heading.classList.contains("datatable-end"))
      .map((heading) => heading.textContent);
    expect(headings).toEqual([
      "Reviewed",
      "Accepted",
      "Edited",
      "Rejected",
      "Undone",
      "Details",
    ]);
    // The transition is a name, read from the start like any name.
    const name = within(table).getByText("Discovery → Negotiation");
    expect(name.closest("td")?.classList.contains("datatable-end")).toBe(false);
  });

  it("names the evidence a transition rested on in words, never by its key", async () => {
    vi.stubGlobal(
      "fetch",
      reportStub({
        window_days: 30,
        data: [
          {
            ...REPORT.data[0],
            evidence_kinds: [
              ...REPORT.data[0].evidence_kinds,
              {
                kind: "meeting_booked",
                reviewed: 3,
                accepted_clean: 2,
                unsafe: 0,
              },
            ],
          },
        ],
      }),
    );
    const user = userEvent.setup();
    render(<StageAutomationCard />);

    await user.click(
      await screen.findByRole("button", {
        name: "Details for Discovery → Negotiation",
      }),
    );

    expect(await screen.findByText("Document signed")).toBeTruthy();
    expect(screen.getByText("12 of 12 accepted as proposed")).toBeTruthy();
    // A kind this build has no label for still reads as words.
    expect(screen.getByText("meeting booked")).toBeTruthy();
    expect(document.body.textContent).not.toMatch(
      /document_signed|meeting_booked/,
    );
  });

  it("tells a transition nobody answered from one contacts reject", async () => {
    vi.stubGlobal("fetch", reportStub());
    const user = userEvent.setup();
    render(<StageAutomationCard />);

    const unanswered = (
      await screen.findByText(/Negotiation → Contract/)
    ).closest("tr");
    expect(unanswered).toBeTruthy();
    // NOT "0%". A rate of zero says contacts refuse it every time; this
    // transition has been proposed four times and answered never, and the two
    // ask for opposite fixes — a better proposal, or somebody to look.
    expect(unanswered?.textContent).not.toContain("0%");
    // The sentence sits under the name; the figure column holds only a dash.
    const cells = Array.from(unanswered?.querySelectorAll("td") ?? []);
    expect(cells[0].textContent).toContain("Proposed, but none reviewed yet.");
    expect(cells[1].textContent).toBe("—");
    // And the four open cards are one press away, so the absence is explained.
    await user.click(
      screen.getByRole("button", {
        name: "Details for Negotiation → Contract",
      }),
    );
    const open = await screen.findByText("Still open");
    expect(open.closest(".factlist-row")?.textContent).toContain("4");
  });

  it("spells out each short heading in a definition list", async () => {
    vi.stubGlobal("fetch", reportStub());
    render(<StageAutomationCard />);

    await screen.findByRole("table", { name: "Stage automation" });
    const terms = Array.from(document.querySelectorAll("dl.factlist dt")).map(
      (term) => term.textContent,
    );
    expect(terms).toEqual([
      "Reviewed",
      "Accepted",
      "Edited",
      "Undone",
      "Expired",
      "Days observed",
    ]);
  });

  // The three states a reader must be able to tell apart. "Nothing proposed" is
  // a fact somebody acts on by waiting; a refused or failed load is not, and
  // drawing the same words for both sends them to wait for numbers that will
  // never come.
  // The safety number is the one held to a CEILING, and the threshold below it
  // is one percent. Rounded to whole numbers, a transition sitting at 0.4%
  // undone prints "0%" and reads as flawless — a rounding that flatters,
  // exactly where flattery decides whether automation keeps running.
  it("keeps a sub-one-percent unsafe rate visible", async () => {
    vi.stubGlobal(
      "fetch",
      reportStub({
        window_days: 30,
        data: [
          {
            ...REPORT.data[0],
            reviewed: 250,
            unsafe: 1,
            unsafe_rate: 0.004,
          },
        ],
      }),
    );
    render(<StageAutomationCard />);

    const row = (await screen.findByText(/Discovery → Negotiation/)).closest(
      "tr",
    );
    expect(row?.textContent).toContain("0.4");
    // The whole-number rounding would have printed this, and it says the
    // opposite of what happened.
    expect(row?.textContent).not.toMatch(/(^|[^.\d])0%/);
  });

  it("does not call a failed load an empty record", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith("/v1/me")) {
          return jsonResponse(
            meFixture({ roles: ["admin"], allow: PIPELINE_ADMIN }),
          );
        }
        if (url.includes("/pipelines")) {
          return jsonResponse({ data: PIPELINES, page: {} });
        }
        return jsonResponse(
          { title: "Server error", status: 500, type: "about:blank" },
          500,
        );
      }),
    );
    render(<StageAutomationCard />);

    // The POSITIVE assertion first, and it is what makes the negative one
    // below mean anything: waiting on an absence resolves before the screen has
    // rendered anything at all, so "the empty sentence is missing" would pass
    // against a blank page.
    await waitFor(() => {
      expect(document.body.textContent).toMatch(/error|failed|wrong/i);
    });
    // Only now is the absence a fact about a settled render.
    expect(screen.queryByText(/has not proposed a stage move/i)).toBeNull();
  });

  it("says so when the pipeline has never been proposed a move", async () => {
    vi.stubGlobal("fetch", reportStub({ window_days: 30, data: [] }));
    render(<StageAutomationCard />);

    expect(
      await screen.findByText(/has not proposed a stage move/i),
    ).toBeTruthy();
  });
});
