// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { stubClipboard } from "../../design-system/clipboard-testing";
import { LocaleProvider } from "../../i18n";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
} from "../../testing/story-utils";
import { MailReviewRequests } from "./mailreviewrequests";
import { HEALTH_AT } from "./systemhealth.fixtures";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

it("offers personal choices only to the current owner and prepares a request for another owner", async () => {
  const user = userEvent.setup();
  const clipboard = stubClipboard("accepts");
  installFetchStub({
    "GET /me": meRoute({ job_health: ["read"] }),
    "GET /admin/capture-health": () =>
      jsonResponse({
        generated_at: HEALTH_AT,
        mailboxes: [],
        sweeps: [],
        connection_concerns: [],
        classifier: { pending: 4, unsure: 3, exhausted: 0 },
        held_meetings: { count: 0 },
        owner_reviews: [
          {
            user_id: "00000000-0000-4000-8000-000000000001",
            display_name: "Alex",
            sender_decisions: 2,
            thread_decisions: 0,
          },
          {
            user_id: "00000000-0000-4000-8000-000000000002",
            display_name: "Maya",
            sender_decisions: 1,
            thread_decisions: 0,
          },
        ],
      }),
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <MailReviewRequests review="senders" />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await user.click(screen.getByRole("button", { name: "Ask a mailbox owner" }));
  const dialog = await screen.findByRole("dialog", {
    name: "Mailbox decisions",
  });
  expect(
    await within(dialog).findByText("2 sender decisions waiting"),
  ).toBeVisible();
  expect(
    within(dialog).getAllByRole("button", { name: "Review my choices" }),
  ).toHaveLength(1);
  await user.click(
    within(dialog).getByRole("button", {
      name: "Copy review request for Maya",
    }),
  );
  await waitFor(() => expect(clipboard.written).toHaveLength(1));
  expect(clipboard.written[0]).toContain(
    "Maya, open Worklist → Decisions and accept or decline the proposed contacts:",
  );
  expect(clipboard.written[0]).toContain("#/home?queue=1&queue_view=decisions");
  expect(clipboard.written[0]).not.toContain(
    "00000000-0000-4000-8000-000000000002",
  );
  await user.click(
    within(dialog).getByRole("button", { name: "Review my choices" }),
  );
  expect(window.location.hash).toBe("#/home?queue=1&queue_view=decisions");
});
