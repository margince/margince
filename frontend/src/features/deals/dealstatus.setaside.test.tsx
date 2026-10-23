// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { DealStatusCardPanel } from "./dealstatus";

// The deal card's move can be set aside in place, and the Undo brings it back.
// A move with no verb of its own is the case that matters. Before "Not now" it
// sat on the card with no control at all.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const DEAL = "01a03000-0000-7000-8000-000000000001";
const FINGERPRINT = "a".repeat(64);
const REASON = "Nobody has been contacted yet. Open with the champion.";

it("sets the move aside, and the Undo puts it back", async () => {
  let setAside = false;
  const restored: string[] = [];
  installFetchStub({
    [`GET /deals/${DEAL}/status`]: () =>
      jsonResponse({
        deal_id: DEAL,
        story: { sentences: [] },
        generated_at: "2026-08-24T00:00:00Z",
        generated_by: "deterministic",
        // Draft with no message to answer: no verb the card can perform.
        next: setAside
          ? undefined
          : {
              action: "draft_email",
              reason: REASON,
              evidence: [],
              fingerprint: FINGERPRINT,
            },
      }),
    [`GET /deals/${DEAL}/coverage`]: () =>
      jsonResponse({
        deal_id: DEAL,
        stakeholders: [],
        risks: [],
        sections_omitted: [],
      }),
    [`POST /deals/${DEAL}/move/dismiss`]: (body) => {
      expect(body).toEqual({ fingerprint: FINGERPRINT });
      setAside = true;
      return new Response(null, { status: 204 });
    },
    [`DELETE /deals/${DEAL}/move/dismiss`]: () => {
      restored.push(FINGERPRINT);
      setAside = false;
      return new Response(null, { status: 204 });
    },
  });
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ToastProvider>
          <DealStatusCardPanel deal={{ id: DEAL, name: "PIM rollout" }} />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  const user = userEvent.setup();

  await screen.findByText(REASON);
  await user.click(
    screen.getByRole("button", { name: en["standing.next.snooze"] }),
  );

  await waitFor(() => expect(screen.queryByText(REASON)).toBeNull());
  await user.click(
    await screen.findByRole("button", { name: en["common.undo"] }),
  );

  await screen.findByText(REASON);
  expect(restored).toEqual([FINGERPRINT]);
});
