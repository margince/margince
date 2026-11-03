/** @vitest-environment happy-dom */
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import { InputCheckAnswerSheet } from "./inputcheck.answer";
import { CallEditorProvider } from "./inputcheck.call";

const session = vi.hoisted(() => ({ userId: "author" }));
vi.mock("../../lib/session", () => ({
  useMe: () => ({ data: { user: { id: session.userId } } }),
}));

type InputCheck = components["schemas"]["InputCheck"];

const callCheck: InputCheck = {
  id: "f1",
  type: "forecast_call_moved",
  subject_kind: "forecast_call",
  subject_id: "c1",
  severity: "medium",
  status: "open",
  owner_id: "author",
  first_seen_at: "2026-10-30T09:00:00Z",
  last_seen_at: "2026-10-30T09:00:00Z",
  inputs_changed: false,
  claim: { scope_kind: "workspace", period_start: "2026-10-01" },
  observed: { comparison: "moved", changed_deals: 2, deal_ids: ["d1", "d2"] },
};

function sheet(open: () => void) {
  render(
    <CallEditorProvider
      editor={{
        open,
        scope: { scope_kind: "workspace" },
        periodStart: "2026-10-01",
      }}
    >
      <InputCheckAnswerSheet
        check={callCheck}
        open
        labels={{ title: "Answer", cancel: "Close", outcomes: [] } as never}
        onSubmit={() => {}}
        onClose={() => {}}
      />
    </CallEditorProvider>,
  );
}

describe("a finding about a manager's call", () => {
  afterEach(() => {
    session.userId = "author";
  });

  // The author answers by calling again, so the sheet offers that and none
  // of the deal answers.
  it("offers its author a new call and no deal answer", async () => {
    const open = vi.fn();
    sheet(open);
    expect(screen.queryByText(/fixed|value is correct/i)).toBeNull();
    await userEvent.click(
      screen.getByRole("button", { name: en["review.submitNewCall"] }),
    );
    expect(open).toHaveBeenCalledOnce();
  });

  // Anybody else reads what moved and who can act, and is offered nothing the
  // server would refuse.
  it("shows anybody else what moved and no action", () => {
    session.userId = "someone-else";
    sheet(vi.fn());
    expect(screen.getByText(en["review.callAuthorOnly"])).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: en["review.submitNewCall"] }),
    ).toBeNull();
  });
});
