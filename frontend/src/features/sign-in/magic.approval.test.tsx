/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { wonProposal } from "../../testing/magic.approval.fixtures";
import { receipt, renderMagic, stub } from "../../testing/magic.testkit";

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

it("omits absent optional evidence without blocking the proposed action", async () => {
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
  expect(await screen.findByText("Mark as won")).toBeTruthy();
  expect(screen.queryByText(/No supporting source/)).toBeNull();
  expect(screen.getByRole("link", { name: "Decide" })).toBeTruthy();
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

it("omits the same approval shown in Focus but keeps another decision for the same deal", async () => {
  stub(
    receipt({
      needs_you: [wonProposal, { ...wonProposal, id: "other-approval" }],
    }),
  );
  renderMagic("en", undefined, { visibleApprovalIds: [wonProposal.id] });
  await screen.findByText("Mark as won");
  expect(screen.getAllByText("Mark as won")).toHaveLength(1);
  expect(screen.getAllByText("Nordwind renewal")).toHaveLength(1);
});

it("keeps an unknown legacy decision reachable without generic waiting prose", async () => {
  stub(
    receipt({
      needs_you: [
        {
          ...wonProposal,
          approval_preview: undefined,
          summary: { key: "magic.action.future_proposal" },
        },
      ],
    }),
  );
  renderMagic();
  expect(await screen.findByText("Review proposed change")).toBeTruthy();
  expect(screen.getByRole("link", { name: "Decide" })).toBeTruthy();
  expect(
    screen.queryByText(/waiting for your word|Nothing happens until/),
  ).toBeNull();
});

it.each([
  ["magic.consequence.capture_not_collecting", "Capture stopped."],
  [
    "magic.consequence.capture_may_be_incomplete",
    "Recent capture may be incomplete.",
  ],
  [
    "magic.consequence.capture_history_incomplete",
    "History import incomplete.",
  ],
])("keeps factual capture coverage %s", async (consequence, status) => {
  stub(
    receipt({
      watching: [
        {
          ...wonProposal,
          lane: "watching",
          summary: {
            key: "magic.action.capture_sync_failing",
            values: { provider: "google" },
          },
          consequence,
        },
      ],
    }),
  );
  renderMagic();
  expect(await screen.findByText(status)).toBeTruthy();
});

it("draws legacy and grouped changes, hiding none of them", async () => {
  stub(
    receipt({
      done: [
        {
          ...wonProposal,
          id: "same-looking-id",
          lane: "done",
          summary: { key: "magic.action.advance_stage" },
        },
        {
          ...wonProposal,
          id: "group-audit",
          lane: "done",
          count: 2,
          receipt_ids: ["known", "other"],
          summary: { key: "magic.action.advance_stage" },
        },
      ],
    }),
  );
  renderMagic();
  await screen.findByText("All 3 changes, one by one");
  expect(screen.getAllByText("A deal moved to its next stage")).toHaveLength(2);
});
