/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { proposal, readingsDay, taskRow } from "../../testing/brief.fixtures";
import { jsonResponse, render, stubApi } from "../../testing/brief.testkit";
import { wonProposal } from "../../testing/magic.approval.fixtures";
import { receipt } from "../../testing/magic.testkit";
import { BriefScreen } from "./brief";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

it("keeps the exact approval review reachable from Focus without a second receipt preview", async () => {
  const user = userEvent.setup();
  const entity = wonProposal.entity;
  if (!entity) throw new Error("The proposal fixture must name its deal");
  const row = {
    ...taskRow(wonProposal.id, "Mark as won"),
    source: "approval" as const,
    category: "decisions" as const,
    kind: "advance_deal",
    subject: { ...entity, type: "deal" as const },
    actions: ["decide" as const],
  };
  const calls = stubApi({
    "GET /worklist": () => jsonResponse(readingsDay({}, [row])),
    "GET /magic": () => jsonResponse(receipt({ needs_you: [wonProposal] })),
    [`GET /approvals/${wonProposal.id}`]: () =>
      jsonResponse(
        proposal(wonProposal.id, "Mark as won", {
          kind: "advance_deal",
          preview: wonProposal.approval_preview,
          target_entity_type: "deal",
          target_entity_id: wonProposal.entity?.id,
          target_label: wonProposal.entity?.label,
          proposed_change: { to_stage_id: "won" },
        }),
      ),
  });
  render(<BriefScreen />);
  const review = await screen.findByRole("button", {
    name: en["worklist.verb.decide"],
  });
  expect(screen.queryByRole("link", { name: "Decide" })).toBeNull();
  expect(screen.getByText("Nordwind renewal")).toBeTruthy();
  await user.click(review);
  const dialog = await screen.findByRole("dialog");
  expect(await within(dialog).findByText("Qualified")).toBeTruthy();
  expect(within(dialog).getAllByText("Won").length).toBeGreaterThan(0);
  expect(within(dialog).getByText("Nordwind Logistics")).toBeTruthy();
  expect(within(dialog).getByText("Dana Weiss")).toBeTruthy();
  expect(dialog.textContent).toContain("125,000");
  expect(calls.filter((call) => call.method !== "GET")).toEqual([]);
});

it("keeps a refused Focus decision reachable through its Magic review link", async () => {
  const user = userEvent.setup();
  const preview = {
    version: 2 as const,
    state: "stale" as const,
    reviewer_reason: "requested_decider" as const,
    changes: [],
  };
  const row = {
    ...taskRow(wonProposal.id, "Mark as won"),
    source: "approval" as const,
    category: "decisions" as const,
    actions: [],
  };
  stubApi({
    "GET /worklist": () => jsonResponse(readingsDay({}, [row])),
    "GET /magic": () =>
      jsonResponse(
        receipt({ needs_you: [{ ...wonProposal, approval_preview: preview }] }),
      ),
    [`GET /approvals/${wonProposal.id}`]: () =>
      jsonResponse(
        proposal(wonProposal.id, "Mark as won", {
          kind: "advance_deal",
          preview,
        }),
      ),
  });
  render(<BriefScreen />);
  const link = await screen.findByRole("link", { name: "Decide" });
  expect(link.getAttribute("href")).toContain(wonProposal.id);
  expect(
    screen.queryByRole("button", { name: en["worklist.verb.decide"] }),
  ).toBeNull();
  await user.click(link);
  const dialog = await screen.findByRole("dialog");
  expect(
    await within(dialog).findByRole("button", { name: en["decision.reject"] }),
  ).toBeTruthy();
});
