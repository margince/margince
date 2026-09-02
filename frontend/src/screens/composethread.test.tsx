/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
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

  const back = screen.getByRole("button", { name: "Choose another" });
  expect(back.classList.contains("btn")).toBe(true);
  await user.click(back);
  expect(leave).toHaveBeenCalledTimes(1);
});
