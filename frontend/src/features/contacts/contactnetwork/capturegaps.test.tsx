/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../../api/schema";
import { LocaleProvider } from "../../../i18n";
import { en } from "../../../i18n/en";
import { CaptureGapsNotice } from "./capturegaps";
import { CONTACT, jsonResponse } from "./network.testkit";

type ContactCaptureGap = components["schemas"]["ContactCaptureGap"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const MISSING = /later mail may be missing/;
const FAILED = en["contact.network.captureGapsFailed"];

function seats(n: number): ContactCaptureGap[] {
  return Array.from({ length: n }, (_, i) => ({
    user_id: `u-${i}`,
    display_name: `Colleague ${i}`,
    invited: false,
    last_on_thread_at: "2026-10-01T10:00:00Z",
  }));
}

function networkAnswer(gaps: ContactCaptureGap[], total: number): Response {
  return jsonResponse({
    contact_id: CONTACT,
    colleagues: [],
    capture_gaps: gaps,
    capture_gaps_total: total,
  });
}

function refusal(): Response {
  return new Response(JSON.stringify({ title: "Unavailable", status: 503 }), {
    status: 503,
    headers: { "content-type": "application/problem+json" },
  });
}

// Each call to /network takes the next answer; the last one repeats.
function stubNetwork(...answers: (() => Promise<Response>)[]) {
  const fetch = vi.fn(async () => {
    const next = answers.length > 1 ? answers.shift() : answers[0];
    if (!next) {
      throw new Error("no answer stubbed");
    }
    return next();
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

function renderNotice() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CaptureGapsNotice contactId={CONTACT} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("the notice of colleagues without a mailbox", () => {
  it("draws nothing while the read is in flight", async () => {
    const fetch = stubNetwork(() => new Promise<Response>(() => {}));
    renderNotice();
    await waitFor(() => expect(fetch).toHaveBeenCalled());
    expect(screen.queryByText(MISSING)).toBeNull();
    expect(screen.queryByText(FAILED)).toBeNull();
  });

  it("says a failed read could not be read, and retries it", async () => {
    const fetch = stubNetwork(
      async () => refusal(),
      async () => networkAnswer(seats(1), 1),
    );
    renderNotice();
    expect(await screen.findByText(FAILED)).toBeTruthy();
    expect(screen.queryByText(MISSING)).toBeNull();

    await userEvent.click(
      screen.getByRole("button", { name: en["common.retry"] }),
    );
    expect(await screen.findByText(MISSING)).toBeTruthy();
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(screen.queryByText(FAILED)).toBeNull();
  });

  it("draws nothing when every colleague on the threads is captured", async () => {
    const fetch = stubNetwork(async () => networkAnswer([], 0));
    renderNotice();
    await waitFor(() => expect(fetch).toHaveBeenCalled());
    await waitFor(() => {
      expect(screen.queryByText(MISSING)).toBeNull();
      expect(screen.queryByText(FAILED)).toBeNull();
    });
  });

  it("names how many more there are when the list stopped short", async () => {
    stubNetwork(async () => networkAnswer(seats(10), 12));
    renderNotice();
    expect(
      await screen.findByText(
        /Colleague 9 and 2 more are on this contact’s threads/,
      ),
    ).toBeTruthy();
  });

  it("names everyone and no more when the list is whole", async () => {
    stubNetwork(async () => networkAnswer(seats(3), 3));
    renderNotice();
    const notice = await screen.findByText(MISSING);
    expect(notice.textContent).toContain(
      "Colleague 0, Colleague 1, Colleague 2 are",
    );
    expect(notice.textContent).not.toMatch(/more/);
  });

  // A server older than capture_gaps_total sends only the list. The notice
  // then names that list and makes up no "and N more".
  it("names the whole list when an older server sends no total", async () => {
    stubNetwork(async () =>
      jsonResponse({
        contact_id: CONTACT,
        colleagues: [],
        capture_gaps: seats(2),
      }),
    );
    renderNotice();
    const notice = await screen.findByText(MISSING);
    expect(notice.textContent).toContain("Colleague 0, Colleague 1 are");
    expect(notice.textContent).not.toMatch(/more/);
  });
});
