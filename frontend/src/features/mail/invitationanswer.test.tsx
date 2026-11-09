// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

// An invitation asks for an RSVP, so every surface that would offer a reply
// offers its answers instead, and a press answers on the reader's calendar.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { LocaleProvider } from "../../i18n";
import { ChannelReplyAction } from "./compose.reply";
import { InvitationAnswer } from "./invitationanswer";

const ACTIVITY = "01a05500-0000-7000-8000-0000000000a1";
const CONTACT = "01a05500-0000-7000-8000-0000000000c1";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function draw(node: ReactNode, respond: (body: unknown) => Response) {
  const calls: { url: string; method: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request) => {
      const body = await input.clone().json();
      calls.push({ url: input.url, method: input.method, body });
      return respond(body);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{node}</LocaleProvider>
    </QueryClientProvider>,
  );
  return calls;
}

function answered(body: unknown) {
  const { response } = body as { response: string };
  return new Response(JSON.stringify({ response }), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

describe("answering an invitation", () => {
  it("draws the answers where a reply would be", () => {
    draw(
      <ChannelReplyAction
        activityId={ACTIVITY}
        kind="email"
        entityType="contact"
        entityId={CONTACT}
        invitation={{ response: "needs_action" }}
      />,
      answered,
    );

    expect(screen.queryByRole("button", { name: "Reply" })).toBeNull();
    for (const name of ["Accept", "Maybe", "Decline"]) {
      expect(
        screen.getByRole("button", { name }).getAttribute("aria-pressed"),
      ).toBe("false");
    }
  });

  it("keeps the reply for a reader who may not read the invitation", () => {
    draw(
      <ChannelReplyAction
        activityId={ACTIVITY}
        kind="email"
        entityType="contact"
        entityId={CONTACT}
        contentWithheld
        invitation={{ response: "needs_action" }}
      />,
      answered,
    );

    expect(screen.queryByRole("button", { name: "Accept" })).toBeNull();
  });

  it("sends the pressed answer and then holds it", async () => {
    const onAnswered = vi.fn();
    const calls = draw(
      <InvitationAnswer
        activityId={ACTIVITY}
        invitation={{ response: "accepted" }}
        onAnswered={onAnswered}
      />,
      answered,
    );
    expect(
      screen
        .getByRole("button", { name: "Accept" })
        .getAttribute("aria-pressed"),
    ).toBe("true");

    fireEvent.click(screen.getByRole("button", { name: "Maybe" }));

    await waitFor(() => expect(onAnswered).toHaveBeenCalledTimes(1));
    expect(calls).toEqual([
      {
        url: expect.stringContaining(
          `/activities/${ACTIVITY}/calendar-response`,
        ),
        method: "PUT",
        body: { response: "tentative" },
      },
    ]);
    expect(
      screen
        .getByRole("button", { name: "Maybe" })
        .getAttribute("aria-pressed"),
    ).toBe("true");
    expect(
      screen
        .getByRole("button", { name: "Accept" })
        .getAttribute("aria-pressed"),
    ).toBe("false");
  });

  it("says why a refused answer did not go, and keeps the one held", async () => {
    draw(
      <InvitationAnswer
        activityId={ACTIVITY}
        invitation={{ response: "needs_action" }}
      />,
      () =>
        new Response(
          JSON.stringify({
            type: "about:blank",
            title: "Validation failed",
            status: 422,
            detail:
              "your connected calendar does not hold this event; answer it in the calendar it was sent to",
          }),
          {
            status: 422,
            headers: { "Content-Type": "application/problem+json" },
          },
        ),
    );

    fireEvent.click(screen.getByRole("button", { name: "Decline" }));

    expect(await screen.findByRole("alert")).toBeTruthy();
    expect(
      screen
        .getByRole("button", { name: "Decline" })
        .getAttribute("aria-pressed"),
    ).toBe("false");
  });
});
