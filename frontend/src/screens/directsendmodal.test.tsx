/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { DirectSendModal } from "./directsendmodal";

// What must be true before an exception is recorded in somebody's name.
//
// Each of these is a way the record could end up asserting something that did
// not happen: a decision nobody acknowledged, a reason nobody gave, or words
// nobody was shown.

const review: components["schemas"]["CommunicationReview"] = {
  id: "review-1",
  state: "needs_repair",
  kind: "single",
  reason_code: "no_marketing_consent",
  refusals: [],
  warning: {
    version: "override-v1",
    text: "The refusal stands and this is recorded.",
  },
};

function open(
  overrides: Partial<React.ComponentProps<typeof DirectSendModal>> = {},
) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <DirectSendModal open onClose={vi.fn()} review={review} {...overrides} />
    </QueryClientProvider>,
  );
}

describe("DirectSendModal", () => {
  // A pre-ticked acknowledgement records a decision nobody made, which is the
  // one thing this record must never say.
  it("never opens with the acknowledgement already ticked", () => {
    open();
    expect((screen.getByRole("checkbox") as HTMLInputElement).checked).toBe(
      false,
    );
  });

  // The words are the server's. Composing them here would record the director
  // as having read the server's caution while showing them ours.
  it("shows the caution the server served, verbatim", () => {
    open();
    expect(
      screen.getByText("The refusal stands and this is recorded."),
    ).toBeTruthy();
  });

  // An older server, or a contract this client is ahead of. Rendering nothing
  // beats inventing words the record would then name.
  it("refuses to record when the server served no caution", () => {
    open({ review: { ...review, warning: undefined } });
    expect(
      screen.queryByText("The refusal stands and this is recorded."),
    ).toBeNull();
    // Nothing to acknowledge, so confirming is refused rather than submitting
    // an empty version the server would reject with an error about a field the
    // director never saw.
    const confirm = screen.getByRole("button", {
      name: /record the exception and send/i,
    }) as HTMLButtonElement;
    expect(confirm.disabled).toBe(true);
  });

  // A director who ticks, cancels, and opens a DIFFERENT refused message must
  // not arrive with the previous acknowledgement standing — that is one click
  // from recording, against this message, a decision they made about another.
  it("forgets the acknowledgement when the message changes", async () => {
    const user = userEvent.setup();
    const { rerender } = open();
    await user.click(screen.getByRole("checkbox"));
    expect((screen.getByRole("checkbox") as HTMLInputElement).checked).toBe(
      true,
    );

    rerender(
      <QueryClientProvider client={new QueryClient()}>
        <DirectSendModal
          open
          onClose={vi.fn()}
          review={{ ...review, id: "review-2" }}
        />
      </QueryClientProvider>,
    );
    expect((screen.getByRole("checkbox") as HTMLInputElement).checked).toBe(
      false,
    );
  });

  // Both halves are required, and the modal says which is missing rather than
  // refusing silently.
  it("will not send until the director has both acknowledged and said why", async () => {
    const user = userEvent.setup();
    open();
    // RE-QUERIED EACH TIME. The button is re-rendered when either half of the
    // precondition changes, so a node captured once is a node that stopped
    // being the one on screen — and the assertion would read a state nobody
    // is looking at.
    const confirm = () =>
      screen.getByRole("button", {
        name: /record the exception and send/i,
      }) as HTMLButtonElement;

    expect(confirm().disabled).toBe(true);

    await user.click(screen.getByRole("checkbox"));
    expect(confirm().disabled).toBe(true);

    await user.type(
      screen.getByLabelText(/why, in your own words/i),
      "The contract obliges it.",
    );
    expect(confirm().disabled).toBe(false);
  });

  // Enter in the reason must not confirm. A human typing a sentence presses it
  // without meaning to record anything, and a single-line input would submit.
  it("takes a newline in the reason rather than confirming", async () => {
    const user = userEvent.setup();
    open();
    const reason = screen.getByLabelText(/why, in your own words/i);
    await user.click(reason);
    await user.keyboard("first{Enter}second");
    expect((reason as HTMLTextAreaElement).value).toContain("\n");
  });
});
