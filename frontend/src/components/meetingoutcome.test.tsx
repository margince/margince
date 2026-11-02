// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { MeetingOutcome } from "./meetingoutcome";

const MEETING_ID = "01a05500-0000-7000-8000-0000000000c1";

type Sent = {
  method: string;
  url: string;
  ifMatch: string | null;
  body: unknown;
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// Records every request the card makes, and answers a PATCH as the server
// does: with the meeting at its next version.
function stubServer(sent: Sent[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : new Request(input);
      const text = await request.clone().text();
      sent.push({
        method: request.method,
        url: new URL(request.url).pathname,
        ifMatch: request.headers.get("If-Match"),
        body: text === "" ? undefined : JSON.parse(text),
      });
      return new Response(
        JSON.stringify({ id: MEETING_ID, meeting_status: "held", version: 8 }),
        { status: 200, headers: { "content-type": "application/json" } },
      );
    }),
  );
}

function renderCard() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <MeetingOutcome
            id={MEETING_ID}
            version={7}
            title="Site visit"
            invalidateKeys={[]}
            cancellable
          />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("MeetingOutcome", () => {
  it("records Held in one press, at the meeting's version, with no dialog", async () => {
    const sent: Sent[] = [];
    stubServer(sent);
    renderCard();

    await userEvent.click(
      screen.getByRole("button", { name: "Held: Site visit" }),
    );

    expect(await screen.findByText("Meeting outcome recorded")).toBeTruthy();
    expect(sent).toEqual([
      {
        method: "PATCH",
        url: `/v1/activities/${MEETING_ID}`,
        ifMatch: "7",
        body: { meeting_status: "held" },
      },
    ]);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("draws Held first, then Add notes, then Mark as canceled", () => {
    stubServer([]);
    renderCard();

    expect(
      screen.getAllByRole("button").map((button) => button.textContent),
    ).toEqual(["Held", "Add notes", "Mark as canceled"]);
  });
});
