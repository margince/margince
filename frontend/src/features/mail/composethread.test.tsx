/** @vitest-environment happy-dom */
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import type { ComposeThread } from "./composeanchor";
import type { Conversation } from "./composethread";
import { ThreadBar, threadCards } from "./composethreadbar";

// The thread bar above the draft, in the states a read leaves it. Folded into
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
  atPrecision: "instant",
  count: 2,
  partial: false,
  memberIds: ["a2", "a1"],
};

type Reads = Readonly<{
  conversations?: readonly Conversation[];
  failed?: boolean;
  retry?: () => void;
  anchorFailed?: boolean;
  retryAnchor?: () => void;
  more?: boolean;
  moreFailed?: boolean;
}>;

// Only the reads the bar draws from; the hook behind it is not under test.
function threadOf(reads: Reads): ComposeThread {
  const recent = {
    conversations: reads.conversations ?? [pricing],
    pending: false,
    failed: reads.failed ?? false,
    retry: reads.retry ?? (() => undefined),
    more: reads.more ?? false,
    loadMore: () => undefined,
    loadingMore: false,
    moreFailed: reads.moreFailed ?? false,
  };
  const fake = {
    anchorRead: {
      activity: undefined,
      pending: false,
      failed: reads.anchorFailed ?? false,
      retry: reads.retryAnchor ?? (() => undefined),
    },
    anchorActivity: undefined,
    conversation: {
      messages: [],
      pending: false,
      failed: false,
      retry: () => undefined,
    },
    anchorUnresolved: false,
    recent,
    listed: recent,
    search: "",
    setSearch: () => undefined,
    answeringLine: "Replying to “Pricing for Q4”",
    splitColumns: true,
  };
  return fake as unknown as ComposeThread;
}

function bar(
  reads: Reads,
  props: Partial<ComponentProps<typeof ThreadBar>> = {},
) {
  return (
    <LocaleProvider initial="en">
      <ThreadBar
        thread={threadOf(reads)}
        answering={undefined}
        recordName="Ada Brandt"
        viewerId={undefined}
        nameOf={() => undefined}
        onSelect={() => undefined}
        disabled={false}
        {...props}
      />
    </LocaleProvider>
  );
}

async function openMenu(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    screen.getByRole("button", { name: /(Continue a thread|Change)$/ }),
  );
  const menu = document.querySelector<HTMLElement>(".compose-threadmenu");
  if (!menu) throw new Error("the thread menu is shut");
  return within(menu);
}

it("keeps the threads on screen as a failure when the record's mail could not be read", async () => {
  const user = userEvent.setup();
  const retry = vi.fn();
  render(bar({ conversations: [], failed: true, retry }));

  const menu = await openMenu(user);
  expect(menu.getByText("This section did not load.")).toBeTruthy();
  await user.click(menu.getByRole("button", { name: "Retry" }));
  expect(retry).toHaveBeenCalledTimes(1);
});

it("draws a failed read of the answered message as a failure with a retry", async () => {
  const user = userEvent.setup();
  const retry = vi.fn();
  render(bar({ anchorFailed: true, retryAnchor: retry }, { answering: "a1" }));

  const menu = await openMenu(user);
  expect(menu.getByText("This section did not load.")).toBeTruthy();
  await user.click(menu.getByRole("button", { name: "Retry" }));
  expect(retry).toHaveBeenCalledTimes(1);
});

// Every control here is the design system's button: the card a reader picks a
// thread with, and the way back out of one they picked. A hand-rolled button
// would miss the shared focus ring and the pending and refusal shapes.
it("offers each thread and the way back as design-system buttons", async () => {
  const user = userEvent.setup();
  const select = vi.fn();
  render(bar({}, { onSelect: select }));

  expect(screen.getByText("New thread")).toBeTruthy();
  let menu = await openMenu(user);
  expect(menu.getByText("Recent threads")).toBeTruthy();
  expect(
    menu.getByRole("searchbox", { name: "Search threads with Ada Brandt" }),
  ).toBeTruthy();
  const card = menu.getByRole("button", { name: /Pricing for Q4/ });
  expect(card.classList.contains("btn")).toBe(true);
  expect(card.getAttribute("aria-pressed")).toBe("false");
  // A new thread heads the list, marked while the draft starts one.
  expect(
    menu
      .getByRole("button", { name: "New thread" })
      .getAttribute("aria-pressed"),
  ).toBe("true");
  await user.click(card);
  expect(select).toHaveBeenCalledWith("a2");

  cleanup();
  render(bar({}, { answering: "a1", onSelect: select }));
  menu = await openMenu(user);
  // An older message of the thread marks the thread it belongs to.
  expect(
    menu
      .getByRole("button", { name: /Pricing for Q4/ })
      .getAttribute("aria-pressed"),
  ).toBe("true");
  const back = menu.getByRole("button", { name: "New email" });
  expect(back.classList.contains("btn")).toBe(true);
  await user.click(back);
  expect(select).toHaveBeenLastCalledWith(null);
});

it("puts a reply to a thread outside the offer at the top, marked", () => {
  const cards = threadCards([pricing], "old-1", {
    occurred_precision: "instant",
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
it("draws a thread whose time the read did not carry", async () => {
  const user = userEvent.setup();
  render(
    bar({
      conversations: [
        {
          ...pricing,
          atIso: "",
          summary: {
            activity_id: "a2",
            subject: "Pricing for Q4",
            occurred_at: "",
            occurred_precision: "instant",
            display_status: "team",
            attachment_count: 0,
            move: "none",
            version: 1,
          },
        },
      ],
    }),
  );
  const menu = await openMenu(user);
  expect(menu.getByRole("button", { name: /Pricing for Q4/ })).toBeTruthy();
});

it("offers older mail when the page read so far holds no thread to offer", async () => {
  const user = userEvent.setup();
  render(bar({ conversations: [], more: true }));

  const menu = await openMenu(user);

  expect(menu.queryByText("No thread matches these words.")).toBeNull();
  expect(menu.getByRole("button", { name: "Show more" })).toBeTruthy();
});

it("keeps the threads read so far when an older page fails", async () => {
  const user = userEvent.setup();
  render(bar({ more: true, moreFailed: true }));

  const menu = await openMenu(user);

  expect(menu.getByRole("button", { name: /Pricing for Q4/ })).toBeTruthy();
  expect(
    menu.getByText(
      "Older threads could not be loaded. Press Show more to try again.",
    ),
  ).toBeTruthy();
  expect(menu.getByRole("button", { name: "Show more" })).toBeTruthy();
});
