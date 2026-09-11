/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DirectSendAction } from "./directsendaction";
import type { SendReview } from "./sendreview";

// Which answer a refused rep is offered is the SERVER's decision, because it is
// the side that knows whether they may overrule the engine. This surface draws
// what it was told and nothing else.

const review = (actions: SendReview["actions"]): SendReview => ({
  reviewId: "review-1",
  actions,
});

function draw(actions: SendReview["actions"]) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <DirectSendAction review={review(actions)} />
    </QueryClientProvider>,
  );
}

describe("DirectSendAction", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("offers the send when the server says this caller may", () => {
    draw(["direct_send"]);
    expect(
      screen.getByRole("button", { name: /send with a recorded exception/i }),
    ).toBeTruthy();
  });

  // A rep who cannot overrule the engine sees the ask instead, which is a
  // different component. Drawing a button here that fails when pressed would
  // be worse than drawing none.
  it("offers nothing when the server offered the ask instead", () => {
    draw(["request_decision"]);
    expect(screen.queryByRole("button")).toBeNull();
  });

  // An agent, or a refusal with no message to decide about.
  it("offers nothing when the server offered nothing", () => {
    draw([]);
    expect(screen.queryByRole("button")).toBeNull();
  });

  // The modal needs the review itself — the held message and the served
  // caution — and opening it over a pending fetch would show an
  // acknowledgement before the warning it exists to be read against arrived.
  it("does not open the modal before the review is in hand", () => {
    draw(["direct_send"]);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  // A review can be resolved between the refusal and the click: the message
  // sent from another tab, a colleague directing it. Opening the confirm form
  // over one walks somebody through an acknowledgement the server then
  // refuses, which is worse than saying plainly that it is already decided.
  it("says so rather than opening a form for a review already decided", async () => {
    const settled = {
      id: "review-1",
      state: "resolved",
      kind: "single",
      reason_code: "no_marketing_consent",
      refusals: [],
      warning: { version: "override-v1", text: "The refusal stands." },
    };
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify(settled), {
            status: 200,
            headers: { "content-type": "application/json" },
          }),
      ),
    );
    const user = userEvent.setup();
    draw(["direct_send"]);
    await user.click(
      screen.getByRole("button", { name: /send with a recorded exception/i }),
    );
    expect(await screen.findByText(/already been decided/i)).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  // A failed fetch must be retryable. Setting `open` again changes nothing
  // when it is already true, so without an explicit refetch the button is a
  // dead end after the first failure.
  it("tries again when the first attempt failed", async () => {
    let attempts = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        attempts += 1;
        return new Response(null, { status: 503 });
      }),
    );
    const user = userEvent.setup();
    draw(["direct_send"]);
    const button = () =>
      screen.getByRole("button", { name: /send with a recorded exception/i });

    await user.click(button());
    // React Query retries on its own, so the count is whatever it has reached
    // when the error settles. What matters is that a SECOND press adds to it:
    // without the explicit refetch, setting `open` again changes nothing and
    // the button is a dead end.
    await vi.waitFor(() => expect(attempts).toBeGreaterThan(0));
    const afterFirst = attempts;

    await user.click(button());
    await vi.waitFor(() => expect(attempts).toBeGreaterThan(afterFirst));
  });
});
