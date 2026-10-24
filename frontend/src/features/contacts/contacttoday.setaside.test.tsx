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
import { useContact360 } from "./contact360";
import { ContactToday } from "./contacttoday";

// The contact page's moment can be set aside in place, and the Undo brings it
// back. The page re-reads after each, so the row leaves and returns because
// the server says so.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type Contact360 = components["schemas"]["Contact360"];
type ContactMoment = components["schemas"]["ContactMoment"];

const CAPTURED = {
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-08-10T09:00:00Z",
  updated_at: "2026-08-10T09:00:00Z",
} as const;

const MOMENT: ContactMoment = {
  claim_key: "moment:gone_quiet",
  evidence_fingerprint: "fp-quiet",
  rule: "gone_quiet",
  headline: "Dana has gone quiet",
  why_now: "Your last message went out 40 days ago.",
  confidence: "observed_fact",
  evidence: [],
  recommended_action: {
    kind: "draft_reply",
    label: "Write to Dana",
    state: "available",
  },
};

function view(moment?: ContactMoment): Contact360 {
  return {
    as_of: "2026-08-13T09:00:00Z",
    contact: { id: "p-1", full_name: "Dana Buyer", ...CAPTURED },
    sections_omitted: [],
    next_steps: { data: [], page: { has_more: false } },
    moment,
  };
}

function Page() {
  const read = useContact360("p-1");
  if (!read.data) return null;
  return (
    <ContactToday
      view={read.data}
      moment={read.data.moment}
      onAction={() => {}}
    />
  );
}

it("sets the moment aside, and the Undo puts it back", async () => {
  let setAside = false;
  const sent: unknown[] = [];
  installFetchStub({
    "GET /contacts/p-1/360": () =>
      jsonResponse(view(setAside ? undefined : MOMENT)),
    "POST /contacts/p-1/moment/dismiss": (body) => {
      sent.push(body);
      setAside = true;
      return new Response(null, { status: 204 });
    },
    "DELETE /contacts/p-1/moment/dismiss": () => {
      setAside = false;
      return new Response(null, { status: 204 });
    },
  });
  // The route table drops the query string, so the Undo's URL is caught here.
  const routed = globalThis.fetch;
  const undoUrls: string[] = [];
  vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    if ((request?.method ?? init?.method) === "DELETE") {
      undoUrls.push(request ? request.url : String(input));
    }
    return routed(input, init);
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

  await screen.findByText(MOMENT.headline);
  await user.click(
    screen.getByRole("button", { name: en["standing.next.snooze"] }),
  );

  await waitFor(() => expect(screen.queryByText(MOMENT.headline)).toBeNull());
  expect(sent).toEqual([
    { claim_key: MOMENT.claim_key, evidence_fingerprint: "fp-quiet" },
  ]);
  await user.click(
    await screen.findByRole("button", { name: en["common.undo"] }),
  );

  await screen.findByText(MOMENT.headline);
  // The Undo names the evidence it undoes, so an old toast cannot clear a
  // newer dismissal of changed evidence.
  const undo = new URL(undoUrls[0] ?? "", "https://margince.test");
  expect(undo.searchParams.get("claim_key")).toBe(MOMENT.claim_key);
  expect(undo.searchParams.get("evidence_fingerprint")).toBe("fp-quiet");
});
