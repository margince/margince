/** @vitest-environment happy-dom */
import { act, cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { usePendingApprovals } from "../automations/approvals.queries";
import { DecisionsSection } from "./brief.decisions";
import { deckItems } from "./brief.decisions.items";
import {
  jsonResponse,
  pendingPage,
  proposal,
  render,
  stubApi,
  writes,
} from "./brief.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it.each([1, 2])(
  "names the exact task creation effect for %s commitments",
  async (count) => {
    stubApi({});
    const user = userEvent.setup();
    const queue = Array.from({ length: count }, (_, index) =>
      proposal(`commitment-${index}`, `Send scope ${index}`, {
        kind: "commitment_task",
        bundle_id: count > 1 ? "bundle" : null,
        proposed_change: {
          summary: `Send scope ${index}`,
          party: "Alex",
          promiser_unresolved: true,
        },
      }),
    );
    render(
      <DecisionsSection
        items={deckItems(queue)}
        nowMs={Date.parse("2026-08-20T06:00:00Z")}
        state="ready"
        onAlreadyDecided={() => undefined}
      />,
    );
    const label =
      count === 1
        ? en["approval.commitment.createTask_one"]
        : en["approval.commitment.createTask_other"].replace(
            "{count}",
            String(count),
          );
    expect(screen.getByRole("button", { name: label })).toBeTruthy();
    await user.click(
      screen.getByRole("button", { name: en["brief.deck.viewDeck"] }),
    );
    expect(await screen.findByText(en["stat.evidence"])).toBeTruthy();
    expect(
      screen.getByText(en["approval.commitment.notFulfilled"]),
    ).toBeTruthy();
  },
);

it("explains the unavailable member of a commitment bundle through the real Brief wiring", async () => {
  stubApi({});
  const user = userEvent.setup();
  const queue = [false, true].map((unavailable, index) =>
    proposal(`commitment-${index}`, `Send scope ${index}`, {
      kind: "commitment_task",
      bundle_id: "bundle",
      assignee_inactive: unavailable,
      proposed_change: { summary: `Send scope ${index}`, party: "Alex" },
    }),
  );
  render(
    <DecisionsSection
      items={deckItems(queue)}
      nowMs={Date.parse("2026-08-20T06:00:00Z")}
      state="ready"
      onAlreadyDecided={() => undefined}
    />,
  );
  await user.click(
    screen.getByRole("button", { name: en["brief.deck.viewDeck"] }),
  );
  expect(
    await screen.findByText(/this account cannot receive tasks/),
  ).toBeTruthy();
  expect(screen.queryByRole("button", { name: /Create .*tasks/ })).toBeNull();
});

function LiveDecisions() {
  const pending = usePendingApprovals();
  return (
    <DecisionsSection
      items={deckItems(pending.data?.data ?? [])}
      nowMs={Date.parse("2026-08-20T06:00:00Z")}
      state={pending.isPending ? "loading" : "ready"}
      onAlreadyDecided={() => undefined}
    />
  );
}

it("sends a valid rejection while returning a stale task acceptance to review", async () => {
  const queue = ["a", "b"].map((id) =>
    proposal(id, `Send scope ${id}`, {
      kind: "commitment_task",
      proposed_change: { summary: `Send scope ${id}` },
    }),
  );
  const calls = stubApi({
    "GET /approvals": () => pendingPage(queue, new Set()),
    "POST /approvals/b/reject": () =>
      jsonResponse({ ...queue[1], status: "rejected" }),
  });
  const user = userEvent.setup();
  const { client } = render(<LiveDecisions />);
  await user.click(
    await screen.findByRole("button", { name: en["brief.deck.viewDeck"] }),
  );
  await user.click(
    screen.getByRole("button", {
      name: en["approval.commitment.createTask_one"],
    }),
  );
  await user.click(screen.getByRole("button", { name: en["decision.reject"] }));
  await act(async () => {
    client.setQueryData(["approvals", "pending"], {
      data: [{ ...queue[0], assignee_inactive: true }, queue[1]],
    });
  });
  await user.click(
    screen.getByRole("button", { name: en["brief.deck.commit"] }),
  );
  await waitFor(() =>
    expect(writes(calls).map((call) => call.path)).toEqual([
      "/approvals/b/reject",
    ]),
  );
  expect(screen.getByText(en["brief.deck.acceptanceChanged"])).toBeTruthy();
});
