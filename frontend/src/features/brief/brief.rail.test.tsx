/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { readingsDay } from "../../testing/brief.fixtures";
import {
  fleetDeal,
  jsonResponse,
  render,
  stubApi,
} from "../../testing/brief.testkit";
import { customerPayments } from "../../testing/customerpayments.fixtures";
import { meFixture } from "../../testing/mefixture";
import type { WorklistItem } from "../worklist/worklist.queries";
import { BriefScreen } from "./brief";

// Context panels keep their own loading and failure states so a failed source
// cannot erase an independently available answer.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

// ── The context rail ──

describe("BriefScreen — the context rail", () => {
  // Brief used to pass `company: ""` for every card here, so every quiet deal on
  // this page claimed to belong to no company at all. The panel resolves the
  // company through the same naming the pipeline board uses.
  it("uses the scoped queue for remaining risks and never reads the deals list", async () => {
    const calls = stubApi({
      "GET /worklist": () =>
        jsonResponse(
          readingsDay({}, [
            ...Array.from({ length: 5 }, (_, i) =>
              taskRow(`t${i}`, `Follow up ${i}`),
            ),
            {
              occurred_precision: "instant",
              id: "risk-1",
              source: "deal_at_risk",
              category: "deals_at_risk",
              level: 3,
              title: "Ostwind refit",
              consequence: "deal_drifts",
              because: [],
              actions: [],
            },
          ]),
        ),
    });
    render(<BriefScreen />);
    expect(
      await screen.findByText("Ostwind refit has gone quiet"),
    ).toBeTruthy();
    expect(calls.some((call) => call.path === "/deals")).toBe(false);
  });

  // A clear watch list is not a panel. The box, the header band and the
  // hairline stood around one grey sentence beside the panels that did have
  // news, so the news was read last. The sentence moves to the rail's quiet
  // panel — the RailQuiet cases at the foot of this file.
  it("draws no watch panel when nothing has gone quiet", async () => {
    stubApi({ "GET /deals": () => jsonResponse({ data: [fleetDeal] }) });
    render(<BriefScreen />);

    await waitFor(() =>
      expect(document.querySelector("[aria-busy='true']")).toBeNull(),
    );
    expect(document.querySelector("#brief-watch")).toBeNull();
  });

  // The panel keeps its box wherever it has something to SAY. Past the end of
  // Brief's one page an empty list is "nothing on the page we read" rather than
  // "nothing has gone quiet", and that caveat is content.
  it("does not duplicate risks in a second panel on a partial page", async () => {
    stubApi({
      "GET /worklist": () =>
        jsonResponse({ ...readingsDay({}, []), next_cursor: "next" }),
    });
    const { container } = render(<BriefScreen />);
    await screen.findByText(en["brief.first.title"]);
    expect(container.querySelector("#brief-watch")).toBeNull();
  });
});

// Every card in the rail reads as one sheet: the same small title and no band
// or rule under it. A card that drew its own band read as part of another page.
it("draws every rail card with the flat head, the payments card included", async () => {
  stubApi({
    "GET /me": () =>
      jsonResponse(
        meFixture({
          allow: { company: ["read"], deal: ["read"], finance: ["read"] },
        }),
      ),
    "GET /finance/customer-payments": () => jsonResponse(customerPayments()),
  });
  render(<BriefScreen />);
  const payments = await screen.findByRole("heading", {
    name: en["payments.title"],
  });
  expect(payments.closest(".brief-rail")).not.toBeNull();
  const heads = [...document.querySelectorAll(".brief-rail .panel-head")];
  expect(heads.length).toBeGreaterThan(1);
  for (const head of heads) {
    expect(head.className, head.textContent ?? "").toContain("panel-head-flat");
  }
});

// ── When a source has nothing to report ──
//
// Four panels of chrome around four grey sentences is what a clear morning used
// to draw, and it cost the panels that DID have news the reader's eye. Each
// panel now collapses on its empty reading and one quiet panel carries a line
// per silent source. The two halves have to agree: a printed "nothing booked"
// over a schedule panel that drew would be worse than either alone, which is
// why the panels and this panel share one predicate each.

function taskRow(id: string, title: string): WorklistItem {
  return {
    occurred_precision: "instant",
    id,
    source: "task",
    level: 2,
    category: "tasks",
    title,
    because: [],
    consequence: "task_slips",
    actions: ["complete", "open"],
  };
}
