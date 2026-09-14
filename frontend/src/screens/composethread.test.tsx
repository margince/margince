/** @vitest-environment happy-dom */
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { ConversationChoices, ThreadPane } from "./composethread";

// The two columns a composer can carry beside the form, in the state a failed
// read leaves them. Folded into "nothing here" they would each tell the reader
// something settled that the composer does not know: an empty conversation,
// or a record with no history to continue.

afterEach(() => {
  cleanup();
});

const nobody = () => undefined;

it("draws a failed thread read as a failure with a retry, not as an empty conversation", async () => {
  const user = userEvent.setup();
  const retry = vi.fn();
  render(
    <LocaleProvider initial="en">
      <ThreadPane
        messages={[]}
        pending={false}
        failed
        onRetry={retry}
        nameOf={nobody}
        named
      />
    </LocaleProvider>,
  );

  expect(screen.getByText("This section did not load.")).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Try again" }));
  expect(retry).toHaveBeenCalledTimes(1);
});

it("keeps the ways in on screen as a failure when the record's mail could not be read", async () => {
  const user = userEvent.setup();
  const retry = vi.fn();
  render(
    <LocaleProvider initial="en">
      <ConversationChoices
        conversations={[]}
        pending={false}
        failed
        onRetry={retry}
        onChoose={() => undefined}
      />
    </LocaleProvider>,
  );

  expect(screen.getByText("This section did not load.")).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Try again" }));
  expect(retry).toHaveBeenCalledTimes(1);
});

// Every control here is the design system's button: the row a reader picks a
// conversation with, and the way back out of one they picked. A hand-rolled
// button would miss the shared focus ring and the pending and refusal shapes.
it("offers each conversation and the way back as design-system buttons", async () => {
  const user = userEvent.setup();
  const choose = vi.fn();
  const leave = vi.fn();
  render(
    <LocaleProvider initial="en">
      <ConversationChoices
        conversations={[
          {
            anchorId: "a1",
            subject: "Pricing for Q4",
            counterparts: "Ada Brandt",
            atIso: "2026-08-20T09:00:00Z",
            count: 3,
            partial: false,
          },
        ]}
        pending={false}
        failed={false}
        onRetry={() => undefined}
        onChoose={choose}
      />
      <ThreadPane
        messages={[]}
        pending={false}
        failed={false}
        onRetry={() => undefined}
        nameOf={nobody}
        named
        onLeave={leave}
      />
    </LocaleProvider>,
  );

  const row = screen.getByRole("button", { name: /Pricing for Q4/ });
  expect(row.classList.contains("btn")).toBe(true);
  await user.click(row);
  expect(choose).toHaveBeenCalledWith("a1");

  const back = screen.getByRole("button", { name: "New email" });
  expect(back.classList.contains("btn")).toBe(true);
  await user.click(back);
  expect(leave).toHaveBeenCalledTimes(1);
});

type Activity = components["schemas"]["Activity"];

function message(
  id: string,
  subject: string,
  displayStatus: "team" | "withheld",
): Activity {
  return {
    id,
    kind: "email",
    subject,
    body: `Full text of ${subject}.\n\nA second paragraph beyond the preview.`,
    thread_key: "pricing",
    occurred_at: "2026-08-20T09:00:00Z",
    created_at: "2026-08-20T09:00:00Z",
    updated_at: "2026-08-20T09:00:00Z",
    source: "manual",
    captured_by: "human:u1",
    is_done: false,
    content_state: displayStatus === "withheld" ? "withheld" : "available",
    email_summary: {
      activity_id: id,
      subject,
      preview: `Full text of ${subject}.`,
      direction: "inbound",
      occurred_at: "2026-08-20T09:00:00Z",
      counterparty: "Ada Brandt",
      display_status: displayStatus,
      move: "none",
      attachment_count: 0,
      version: 1,
    },
  };
}

// Reading a message and answering it are two different moves on one row. The
// row picks what the reply answers; the quiet link under it opens the words in
// place, so re-reading the exchange neither retargets the draft nor covers it
// with a second drawer. One message stands open at a time: opening the next
// folds the last, so the pane stays the list it was.
it("opens a readable message's words under its row without retargeting the reply", async () => {
  const user = userEvent.setup();
  const select = vi.fn();
  render(
    <LocaleProvider initial="en">
      <ThreadPane
        messages={[
          message("m1", "Re: Pricing", "team"),
          message("m2", "Re: Delivery", "team"),
          message("m3", "Re: Terms", "withheld"),
        ]}
        selectedId="m1"
        onSelect={select}
        pending={false}
        failed={false}
        onRetry={() => undefined}
        nameOf={nobody}
        named
      />
    </LocaleProvider>,
  );

  const pricing = screen.getByRole("listitem", { name: /Re: Pricing/ });
  const delivery = screen.getByRole("listitem", { name: /Re: Delivery/ });
  expect(
    screen.queryByText(/A second paragraph beyond the preview\./),
  ).toBeNull();

  // Opened, the words stand under the row and the same link folds them back.
  const read = within(delivery).getByRole("button", { name: "Read it" });
  expect(read.classList.contains("btn")).toBe(true);
  await user.click(read);
  expect(
    within(delivery).getByText(/A second paragraph beyond the preview\./),
  ).toBeTruthy();
  expect(
    within(delivery).getByRole("button", { name: "Show less" }),
  ).toBeTruthy();
  // Opening the words was a read, not a pick.
  expect(select).not.toHaveBeenCalled();

  // The way to the whole message is offered beside the fold-back, as the
  // design system's button, and only while the words are open.
  expect(
    within(delivery).getByRole("button", { name: "Read full email" }),
  ).toBeTruthy();
  expect(
    within(pricing).queryByRole("button", { name: "Read full email" }),
  ).toBeNull();

  // The next message opened folds the last.
  await user.click(within(pricing).getByRole("button", { name: "Read it" }));
  expect(
    within(pricing).getByText(/A second paragraph beyond the preview\./),
  ).toBeTruthy();
  expect(
    within(delivery).queryByText(/A second paragraph beyond the preview\./),
  ).toBeNull();
  await user.click(within(pricing).getByRole("button", { name: "Show less" }));
  expect(
    screen.queryByText(/A second paragraph beyond the preview\./),
  ).toBeNull();

  // A withheld message keeps its row and gets no words to open: a link that
  // opens nothing is a claim there was nothing to say.
  expect(
    within(screen.getByRole("listitem", { name: /Re: Terms/ })).queryByRole(
      "button",
      { name: "Read it" },
    ),
  ).toBeNull();
});
