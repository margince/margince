/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { ToastProvider } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { eyebrowKeyFor } from "../../lib/worklisteyebrow";
import { meFixture } from "../../testing/mefixture";
import { DomainQuestionAnswer } from "./worklist.domainquestion";
import type { WorklistItem } from "./worklist.queries";
import { WorklistRow } from "./worklist.row";

// A company question as the Worklist draws it: through the real row, with the
// verbs the server sends, and the focused review its row opens.

type Question = components["schemas"]["DomainQuestion"];
type Context = components["schemas"]["DomainQuestionContext"];

const alex = {
  id: "01a1a000-0000-7000-8000-00000000a1e7",
  name: "Alex Example",
};
const suggested: Context = {
  contacts: [alex],
  contact_count: 2,
  hidden_senders: false,
  sender_address: "alex@alias.example",
  origin_activity_id: "01a1a000-0000-7000-8000-0000000000ac",
  suggested_company: {
    id: "01a1a000-0000-7000-8000-0000000000c0",
    name: "Example Company",
    basis: "redirect_target",
  },
};
const forwardedQuestion: Question = {
  domain: "alias.example",
  deferred: false,
  reason_code: "forwarded_identity",
  asked_at: "2026-10-01T10:00:00Z",
  effective_url: "https://www.example-company.example/",
  redirect_chain: [
    "https://alias.example/",
    "https://www.example-company.example/",
  ],
  domain_context: suggested,
};

function row(over: Partial<WorklistItem> = {}): WorklistItem {
  return {
    occurred_precision: "instant",
    id: "alias.example",
    source: "domain_question",
    category: "decisions",
    level: 6,
    consequence: "data_drifts",
    title: "alias.example",
    because: [],
    dispositions: ["handled"],
    // What the server sends for a redirected site: never keep.
    actions: ["discard", "reopen"],
    domain_reason_code: "forwarded_identity",
    domain_context: suggested,
    ...over,
  };
}

function response(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function transport(
  handle: (request: Request, url: URL) => Response | Promise<Response>,
) {
  vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
    const request =
      input instanceof Request ? input : new Request(String(input));
    const url = new URL(request.url);
    if (url.pathname.endsWith("/me"))
      return response(
        meFixture({ allow: { company: ["read", "update", "create"] } }),
      );
    return handle(request, url);
  });
}

function mount(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>{node}</ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("draws a redirected question's answers through the real row without keep", async () => {
  transport(() => response({ data: [], page: { has_more: false } }));
  mount(<WorklistRow item={row()} position={1} owner="" />);
  expect(
    await screen.findByRole("button", { name: "Choose another" }),
  ).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "Stop capturing in your mailboxes" }),
  ).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Create company" })).toBeNull();
  expect(
    screen.getByText(
      /Which company do Alex Example and one other contact work for\?/,
    ),
  ).toBeTruthy();
  expect(screen.getByRole("button", { name: "Not now" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Done" })).toBeNull();
});

it("confirms the suggested company by sending its id", async () => {
  const user = userEvent.setup();
  const writes: { path: string; body: unknown }[] = [];
  transport(async (request, url) => {
    writes.push({ path: url.pathname, body: await request.json() });
    return response({}, 200);
  });
  mount(<DomainQuestionAnswer item={row()} />);
  expect(
    await screen.findByText(/Confirming links alias.example and its/),
  ).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Confirm company" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(
    writes[0].path.endsWith("/capture/domain-questions/alias.example/keep"),
  ).toBe(true);
  expect(writes[0].body).toEqual({
    company_id: suggested.suggested_company?.id,
  });
});

it("offers only choose and create when nothing is suggested", async () => {
  transport(() => response(null, 202));
  mount(
    <DomainQuestionAnswer
      item={row({
        actions: ["keep", "discard", "reopen"],
        domain_reason_code: "unevidenced",
        domain_context: {
          contacts: [],
          contact_count: 0,
          hidden_senders: false,
          site_read: "unread",
        },
      })}
    />,
  );
  expect(
    await screen.findByRole("button", { name: "Choose company" }),
  ).toBeTruthy();
  expect(screen.getByRole("button", { name: "Create company" })).toBeTruthy();
  expect(screen.queryByText(/Suggested company/)).toBeNull();
  expect(screen.queryByRole("button", { name: "Confirm company" })).toBeNull();
});

it("opens only its own question and keeps website details closed", async () => {
  const user = userEvent.setup();
  const asked: (string | null)[] = [];
  transport((_request, url) => {
    asked.push(url.searchParams.get("domain"));
    return response({ data: [forwardedQuestion], page: { has_more: false } });
  });
  mount(<DomainQuestionAnswer item={row()} />);
  await user.click(
    await screen.findByRole("button", { name: "Choose another" }),
  );
  expect(
    await screen.findByRole("heading", { name: "Company question" }),
  ).toBeTruthy();
  expect(asked).toEqual(["alias.example"]);
  expect(
    screen.getAllByText(/Suggested company: Example Company\./),
  ).toHaveLength(2);
  expect(screen.getByRole("link", { name: "Alex Example" })).toBeTruthy();
  const chain = screen.getByText(
    "https://alias.example/ → https://www.example-company.example/",
  );
  expect(chain.closest("details")?.open).toBe(false);
});

it("says a site that could not be read was not read", async () => {
  const user = userEvent.setup();
  transport(() =>
    response({
      data: [
        {
          domain: "unknown.example",
          deferred: false,
          reason_code: "unevidenced",
          asked_at: "2026-10-01T10:00:00Z",
          domain_context: {
            contacts: [],
            contact_count: 1,
            hidden_senders: true,
            site_read: "unread",
          },
        },
      ],
      page: { has_more: false },
    }),
  );
  mount(
    <DomainQuestionAnswer
      item={row({
        id: "unknown.example",
        actions: ["keep", "discard"],
        domain_reason_code: "unevidenced",
        domain_context: undefined,
      })}
    />,
  );
  await user.click(
    await screen.findByRole("button", { name: "Choose company" }),
  );
  expect(
    await screen.findByText(/Their website could not be read/),
  ).toBeTruthy();
  expect(screen.getByText(/with a contact you cannot see/)).toBeTruthy();
  expect(
    screen.getByText("Which company does unknown.example belong to?"),
  ).toBeTruthy();
});

it("names a company question in the row's eyebrow", () => {
  expect(eyebrowKeyFor(row())).toBe("worklist.source.domain_question");
});
