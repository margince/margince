/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { ConversationRail, railCards } from "./composerail";
import type { Conversation } from "./composethread";

// The rail beside the draft, in the states a read leaves it. Folded into
// "nothing here" a failure would tell the reader something settled the
// composer does not know: a record with no history to continue.

afterEach(() => {
  cleanup();
});

const pricing: Conversation = {
  anchorId: "a2",
  subject: "Pricing for Q4",
  counterparts: "Ada Brandt",
  atIso: "2026-08-20T09:00:00Z",
  count: 2,
  partial: false,
  memberIds: ["a2", "a1"],
};

function rail(props: Partial<ComponentProps<typeof ConversationRail>>) {
  return (
    <LocaleProvider initial="en">
      <ConversationRail
        recordName="Ada Brandt"
        conversations={[pricing]}
        pending={false}
        failed={false}
        onRetry={() => undefined}
        answering={undefined}
        anchor={undefined}
        anchorPending={false}
        anchorFailed={false}
        onRetryAnchor={() => undefined}
        thread={{
          messages: [],
          pending: false,
          failed: false,
          retry: () => undefined,
        }}
        viewerUserId={undefined}
        nameOf={() => undefined}
        onChoose={() => undefined}
        onLeave={() => undefined}
        disabled={false}
        {...props}
      />
    </LocaleProvider>
  );
}

it("keeps the threads on screen as a failure when the record's mail could not be read", async () => {
  const user = userEvent.setup();
  const retry = vi.fn();
  render(rail({ conversations: [], failed: true, onRetry: retry }));

  expect(screen.getByText("This section did not load.")).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Retry" }));
  expect(retry).toHaveBeenCalledTimes(1);
});

it("draws a failed read of the answered message as a failure with a retry", async () => {
  const user = userEvent.setup();
  const retry = vi.fn();
  render(rail({ answering: "a1", anchorFailed: true, onRetryAnchor: retry }));

  expect(screen.getByText("This section did not load.")).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Retry" }));
  expect(retry).toHaveBeenCalledTimes(1);
});

// Every control here is the design system's button: the card a reader picks a
// thread with, and the way back out of one they picked. A hand-rolled button
// would miss the shared focus ring and the pending and refusal shapes.
it("offers each thread and the way back as design-system buttons", async () => {
  const user = userEvent.setup();
  const choose = vi.fn();
  const leave = vi.fn();
  render(rail({ onChoose: choose }));

  expect(
    screen.getByRole("region", { name: "Threads with Ada Brandt" }),
  ).toBeTruthy();
  const card = screen.getByRole("button", { name: /Pricing for Q4/ });
  expect(card.classList.contains("btn")).toBe(true);
  expect(card.getAttribute("aria-pressed")).toBe("false");
  await user.click(screen.getByRole("button", { name: "Reply in thread" }));
  expect(choose).toHaveBeenCalledWith("a2");
  expect(screen.queryByRole("button", { name: "New email" })).toBeNull();

  cleanup();
  render(rail({ answering: "a1", onLeave: leave }));
  // An older message of the thread marks the thread it belongs to.
  expect(
    screen
      .getByRole("button", { name: /Pricing for Q4/ })
      .getAttribute("aria-pressed"),
  ).toBe("true");
  const back = screen.getByRole("button", { name: "New email" });
  expect(back.classList.contains("btn")).toBe(true);
  await user.click(back);
  expect(leave).toHaveBeenCalledTimes(1);
});

it("puts a reply to a thread outside the offer at the top, marked", () => {
  const cards = railCards([pricing], "old-1", {
    id: "old-1",
    kind: "email",
    subject: "An older thread",
    occurred_at: "2026-01-01T09:00:00Z",
    is_done: false,
    source: "capture",
    captured_by: "system:capture",
    created_at: "2026-01-01T09:00:00Z",
    updated_at: "2026-01-01T09:00:00Z",
  });
  expect(cards.map((card) => [card.anchorId, card.selected])).toEqual([
    ["old-1", true],
    ["a2", false],
  ]);
});

// A read that carries no time draws the card without one, rather than taking
// the composer down with an invalid date.
it("draws a thread whose time the read did not carry", () => {
  render(
    rail({
      conversations: [
        {
          ...pricing,
          atIso: "",
          summary: {
            activity_id: "a2",
            subject: "Pricing for Q4",
            occurred_at: "",
            display_status: "team",
            attachment_count: 0,
            move: "none",
            version: 1,
          },
        },
      ],
    }),
  );
  expect(screen.getByRole("button", { name: /Pricing for Q4/ })).toBeTruthy();
});
