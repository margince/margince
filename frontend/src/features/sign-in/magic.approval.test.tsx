/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { wonProposal } from "./magic.approval.fixtures";
import { receipt, renderMagic, stub } from "./magic.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("shows the action, deal once, commercial context and evidence in the Home row", async () => {
  stub(receipt({ needs_you: [wonProposal] }));
  renderMagic();
  const row = (await screen.findByText("Mark as won")).closest("li");
  expect(row).not.toBeNull();
  if (!row) throw new Error("Missing decision row");
  expect(within(row).getAllByText("Nordwind renewal")).toHaveLength(1);
  expect(within(row).getByText("Qualified → Won")).toBeTruthy();
  expect(within(row).getByText("Nordwind Logistics")).toBeTruthy();
  expect(within(row).getByText("Dana Weiss")).toBeTruthy();
  expect(row.textContent).toContain("125,000");
  expect(row.textContent).toContain("We confirm the renewal");
  expect(row.textContent).not.toContain('Close deal "');
  expect(row.textContent).not.toContain("Determined at execution");
  expect(within(row).getByRole("link", { name: "Decide" })).toBeTruthy();
});

it("shows a stale preview warning without leaking cached commercial context", async () => {
  stub(
    receipt({
      needs_you: [
        {
          ...wonProposal,
          approval_preview: {
            version: 2,
            state: "stale",
            reviewer_reason: "requested_decider",
            changes: [],
          },
        },
      ],
    }),
  );
  renderMagic();
  await screen.findByText(
    "This record changed after the proposal. Request a new proposal before approving.",
  );
  expect(screen.queryByText("Nordwind Logistics")).toBeNull();
  expect(screen.queryByText("We confirm the renewal")).toBeNull();
});

it("states when the proposed action has no supporting source", async () => {
  const preview = wonProposal.approval_preview;
  if (!preview) throw new Error("Missing fixture preview");
  stub(
    receipt({
      needs_you: [
        { ...wonProposal, approval_preview: { ...preview, evidence: [] } },
      ],
    }),
  );
  renderMagic();
  await screen.findByText(
    "No supporting source was attached. Check the deal before approving.",
  );
});

it("keeps a task's action and assignee visible when it has a typed preview", async () => {
  stub(
    receipt({
      needs_you: [
        {
          ...wonProposal,
          approval_kind: "commitment_task",
          approval_preview: {
            version: 2,
            state: "ready",
            reviewer_reason: "requested_decider",
            changes: [],
            details: [
              {
                field: "summary",
                value: { kind: "text", text: "Send the revised proposal" },
              },
              {
                field: "assignee_id",
                value: { kind: "owner", text: "Dana Weiss" },
              },
            ],
            evidence: [],
          },
        },
      ],
    }),
  );
  renderMagic();
  expect(await screen.findByText("Send the revised proposal")).toBeTruthy();
  expect(screen.getByText("Dana Weiss")).toBeTruthy();
  expect(screen.queryByText("Review proposed change")).toBeNull();
});
