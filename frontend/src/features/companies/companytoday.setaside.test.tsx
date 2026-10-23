// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { useCompany360 } from "./company360";
import { CompanyNextBar } from "./companynextbar";

// The account's moment can be snoozed from the Next bar, by the same
// fingerprint store as the advice rows. The Undo brings it back.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type Company360 = components["schemas"]["Company360"];
type Moment = NonNullable<Company360["moment"]>;

const FINGERPRINT = "b".repeat(64);

const PROMISE: Moment = {
  claim_key: "moment:account_promise_task:a-2",
  evidence_fingerprint: FINGERPRINT,
  rule: "open_promise",
  headline: "You owe them: Send the signed contract",
  why_now: "Due in 2 days.",
  confidence: "observed_fact",
  evidence: [{ type: "task", id: "a-2", label: "Send the signed contract" }],
  recommended_action: {
    kind: "complete_task",
    label: "Open it from the task list",
    state: "blocked",
  },
};

const QUIET: Moment = {
  claim_key: "moment:nothing_needed",
  evidence_fingerprint: "quiet",
  rule: "nothing_needed",
  headline: "Nothing is owed to this account",
  why_now: "No promise to this account is open or coming due.",
  confidence: "observed_fact",
  evidence: [],
  recommended_action: {
    kind: "log_activity",
    label: "Log something",
    state: "will_confirm",
  },
};

function view(moment: Moment): Company360 {
  return {
    as_of: "2026-08-07T09:00:00Z",
    company: {
      id: "o-1",
      display_name: "Acme",
      source: "manual",
      captured_by: "human:test",
      created_at: "2026-08-01T09:00:00Z",
      updated_at: "2026-08-01T09:00:00Z",
    },
    sections_omitted: [],
    moment,
  };
}

function Page() {
  const read = useCompany360("o-1");
  return read.data ? (
    <CompanyNextBar view={read.data} onOpenTasks={() => {}} />
  ) : null;
}

it("sets the account's moment aside, and the Undo puts it back", async () => {
  let setAside = false;
  const sent: unknown[] = [];
  installFetchStub({
    "GET /companies/o-1/360": () =>
      jsonResponse(view(setAside ? QUIET : PROMISE)),
    "POST /companies/o-1/suggestions/dismiss": (body) => {
      sent.push(body);
      setAside = true;
      return new Response(null, { status: 204 });
    },
    "DELETE /companies/o-1/suggestions/dismiss": () => {
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
          <Page />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  const user = userEvent.setup();

  await screen.findByText(PROMISE.headline);
  await user.click(
    screen.getByRole("button", { name: en["standing.next.snooze"] }),
  );

  await waitFor(() => expect(screen.queryByText(PROMISE.headline)).toBeNull());
  expect(sent).toEqual([{ fingerprint: FINGERPRINT }]);
  await user.click(
    await screen.findByRole("button", { name: en["common.undo"] }),
  );

  await screen.findByText(PROMISE.headline);
});
