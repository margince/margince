// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { en as englishCopy } from "../../i18n/en";
import { jsonResponse, stubRoutes } from "./compose.testkit";
import { DirectSendModal } from "./directsendmodal";

// A message held in the scheduled list may go anyway at a later moment. The
// decision is sent with the moment, and the dialog waits for a whole moment.

const review: components["schemas"]["CommunicationReview"] = {
  id: "review-1",
  state: "needs_context",
  reason_code: "marketing_objection",
  refusals: [],
  warning: { version: "override-v2", text: "The refusal stands." },
};

const DECIDE = "POST /communication-reviews/review-1/direct-send";

function open(onSent = vi.fn()) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <DirectSendModal
        open
        laterZone="Europe/Berlin"
        onClose={vi.fn()}
        onSent={onSent}
        review={review}
      />
    </QueryClientProvider>,
  );
  return onSent;
}

// The moments below are judged against this clock, not the machine's.
beforeEach(() => {
  vi.useFakeTimers({
    now: new Date("2030-01-15T10:00:00Z"),
    toFake: ["Date", "setInterval", "clearInterval"],
  });
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("Send anyway at a later moment", () => {
  it("sends the decision with the moment and reports it scheduled", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      [DECIDE]: () => jsonResponse({ id: "s-1", status: "scheduled" }, 202),
    });
    const onSent = open();
    await user.click(screen.getByRole("checkbox"));
    await user.type(screen.getByLabelText(/^explanation$/i), "They asked.");
    await user.click(screen.getByRole("radio", { name: /at a later time/i }));
    const confirm = () =>
      screen.getByRole("button", { name: /schedule send/i });
    expect(screen.queryByRole("button", { name: /schedule send/i })).toBeNull();

    await user.type(screen.getByLabelText(/^date$/i), "2031-03-04{Enter}");
    await user.type(screen.getByLabelText(/^time$/i), "09:30{Enter}");
    await user.click(confirm());

    await waitFor(() => expect(onSent).toHaveBeenCalledWith("scheduled"));
    const body = sent.find((entry) => entry.key === DECIDE)?.body as Record<
      string,
      unknown
    >;
    expect(body.scheduled_intent).toMatchObject({
      wall_clock: "2031-03-04T09:30",
    });
    expect(body.scheduled_at).toEqual(expect.any(String));
    expect(body.scheduled_tz).toBe("Europe/Berlin");
  });

  it("will not confirm a later send without a whole moment", async () => {
    const user = userEvent.setup();
    stubRoutes();
    open();
    await user.click(screen.getByRole("checkbox"));
    await user.type(screen.getByLabelText(/^explanation$/i), "They asked.");
    await user.click(screen.getByRole("radio", { name: /at a later time/i }));
    await user.type(screen.getByLabelText(/^date$/i), "2031-03-04{Enter}");
    const confirm = screen.getByRole("button", {
      name: /send email/i,
    }) as HTMLButtonElement;
    expect(confirm.disabled).toBe(true);
  });

  it("refuses a moment already past", async () => {
    const user = userEvent.setup();
    stubRoutes();
    open();
    await user.click(screen.getByRole("checkbox"));
    await user.type(screen.getByLabelText(/^explanation$/i), "They asked.");
    await user.click(screen.getByRole("radio", { name: /at a later time/i }));
    await user.type(screen.getByLabelText(/^date$/i), "2020-03-04{Enter}");
    await user.type(screen.getByLabelText(/^time$/i), "09:30{Enter}");
    const confirm = screen.getByRole("button", {
      name: /schedule send/i,
    }) as HTMLButtonElement;
    expect(
      confirm.getAttribute("aria-disabled") ?? String(confirm.disabled),
    ).toBe("true");
    expect(screen.getByText(englishCopy["compose.schedulePast"])).toBeTruthy();
  });

  it("sends now with no moment when Now stays chosen", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      [DECIDE]: () => jsonResponse({ id: "act-1" }, 201),
    });
    const onSent = open();
    await user.click(screen.getByRole("checkbox"));
    await user.type(screen.getByLabelText(/^explanation$/i), "They asked.");
    await user.click(screen.getByRole("button", { name: /send email/i }));
    await waitFor(() => expect(onSent).toHaveBeenCalledWith("sent"));
    expect(sent.find((entry) => entry.key === DECIDE)?.body).not.toHaveProperty(
      "scheduled_at",
    );
  });
});
