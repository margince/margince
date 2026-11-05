// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The deal card's follow-up after a meeting: one message to everyone it was
// with, drafted from the card's own button.

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { draftButton } from "../../testing/composedraft.testkit";
import { MoveButton } from "./movebutton";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const [MEETING, FIRST, SECOND] = [
  "01a05500-0000-7000-8000-0000000000e1",
  "01a05500-0000-7000-8000-0000000000d1",
  "01a05500-0000-7000-8000-0000000000d2",
];

it("drafts the follow-up to everyone the meeting was with", async () => {
  const drafts: { url: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = String(request ? request.url : input);
      if (url.includes("/draft-email")) {
        drafts.push({
          url,
          body: request
            ? await request.clone().json()
            : JSON.parse(String(init?.body)),
        });
        return Response.json({
          subject: "Thanks for the review",
          body: "Hello both,",
          generated_by: "deterministic",
        });
      }
      return Response.json({ data: [] });
    }),
  );
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <MoveButton
          dealId="01a05500-0000-7000-8000-0000000000b1"
          move={{
            action: "draft_email",
            arguments: {
              activity_id: MEETING,
              links: [
                { entity_type: "contact", entity_id: FIRST },
                { entity_type: "contact", entity_id: SECOND },
              ],
            },
          }}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );

  await userEvent.click(
    screen.getByRole("button", { name: /draft follow-up/i }),
  );
  await userEvent.click(await draftButton());
  await screen.findByDisplayValue("Thanks for the review");
  expect(drafts[0]?.url).toContain(`/contacts/${FIRST}/draft-email`);
  expect(drafts[0]?.body).toMatchObject({
    also_to: [SECOND],
    about_activity_id: MEETING,
  });
});
