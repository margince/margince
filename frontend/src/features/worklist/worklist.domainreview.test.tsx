/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { ToastProvider } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { DomainQuestionAnswer } from "./worklist.domainquestion";
import { DomainReviewLauncher } from "./worklist.domainreview";
import type { WorklistItem } from "./worklist.queries";
import { rowAnswer } from "./worklist.row.answer";

const briefAnswer = {
  working: false,
  offered: () => false,
  answer: () => {
    throw new Error("A company question must not answer a brief");
  },
};

type Question = components["schemas"]["DomainQuestion"];
const forwarded: Question = {
  domain: "alias.example",
  deferred: false,
  reason_code: "forwarded_identity",
  asked_at: "2026-10-01T10:00:00Z",
  evidence_at: "2026-09-20T10:00:00Z",
  effective_url: "https://destination.example/",
  redirect_chain: ["https://alias.example/", "https://destination.example/"],
};
const undecided: Question = {
  domain: "unreadable.example",
  deferred: false,
  reason_code: "unevidenced",
  asked_at: "2026-10-01T10:00:00Z",
};
function response(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
function mount(node: ReactNode = <DomainReviewLauncher />) {
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
  return client;
}
function transport(
  handle: (request: Request, url: URL) => Response | Promise<Response>,
  canEdit = true,
) {
  vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
    const request =
      input instanceof Request ? input : new Request(String(input));
    const url = new URL(request.url);
    if (url.pathname.endsWith("/me"))
      return response(
        meFixture({
          allow: { company: canEdit ? ["read", "update", "create"] : ["read"] },
        }),
      );
    return handle(request, url);
  });
}
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

it("opens empty maintenance review even when the queue has no question, then closes", async () => {
  const user = userEvent.setup();
  transport(() => response({ data: [], page: { has_more: false } }));
  mount();
  await user.click(screen.getByRole("button", { name: "Domain review" }));
  expect(await screen.findByText("No domains need review.")).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Close" }));
  expect(screen.queryByRole("dialog")).toBeNull();
});

it("pages all questions and explains forwarded and undated evidence", async () => {
  const user = userEvent.setup();
  const cursors: (string | null)[] = [];
  transport((_request, url) => {
    cursors.push(url.searchParams.get("cursor"));
    return response(
      url.searchParams.has("cursor")
        ? { data: [undecided], page: { has_more: false } }
        : {
            data: [forwarded],
            page: { has_more: true, next_cursor: "next-page" },
          },
    );
  });
  mount();
  await user.click(screen.getByRole("button", { name: "Domain review" }));
  expect(await screen.findByText("alias.example")).toBeTruthy();
  expect(
    screen.getByText("Answering site: https://destination.example/"),
  ).toBeTruthy();
  const chain = screen.getByText(
    "https://alias.example/ → https://destination.example/",
  );
  expect(chain.closest("details")?.open).toBe(false);
  await user.click(screen.getAllByText("Website details")[0]);
  await waitFor(() => expect(chain.closest("details")?.open).toBe(true));
  expect(screen.getByText(/Latest evidence:/)).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Create company" })).toBeNull();
  await user.click(screen.getByRole("button", { name: "Load more" }));
  expect(await screen.findByText("unreadable.example")).toBeTruthy();
  expect(screen.getByText("Evidence date unavailable")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Load more" })).toBeNull();
  expect(cursors).toEqual([null, "next-page"]);
});

it("requires and submits an explicit company match for forwarded identity", async () => {
  const user = userEvent.setup();
  const writes: unknown[] = [];
  transport(async (request, url) => {
    if (url.pathname.endsWith("/companies")) {
      expect(url.searchParams.get("q")).toBe("Known");
      return response({
        data: [
          {
            id: "00000000-0000-4000-8000-000000000001",
            display_name: "Known company",
          },
        ],
      });
    }
    if (request.method === "POST") {
      expect(
        url.pathname.endsWith("/capture/domain-questions/alias.example/keep"),
      ).toBe(true);
      writes.push(await request.json());
      return response(null, 202);
    }
    return response({ data: [forwarded], page: { has_more: false } });
  });
  mount();
  await user.click(screen.getByRole("button", { name: "Domain review" }));
  const search = await screen.findByRole("searchbox", {
    name: "Match company",
  });
  await waitFor(() => expect(search.hasAttribute("disabled")).toBe(false));
  expect(
    screen
      .getByRole("button", { name: "Match company" })
      .hasAttribute("disabled"),
  ).toBe(true);
  vi.useFakeTimers();
  fireEvent.change(search, { target: { value: "Known" } });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(250);
  });
  vi.useRealTimers();
  await user.click(
    await screen.findByRole("button", { name: /Known company/ }),
  );
  await user.click(screen.getByRole("button", { name: "Match company" }));
  await waitFor(() =>
    expect(writes).toEqual([
      { company_id: "00000000-0000-4000-8000-000000000001" },
    ]),
  );
});

it("sends deferral and restoration for the selected question", async () => {
  const user = userEvent.setup();
  let row = undecided;
  const writes: { method: string; path: string; body: unknown }[] = [];
  transport(async (request, url) => {
    if (request.method !== "GET") {
      writes.push({
        method: request.method,
        path: url.pathname,
        body: await request.json(),
      });
      row = { ...row, deferred: request.method === "PUT" };
      return new Response(null, { status: 204 });
    }
    return response({ data: [row], page: { has_more: false } });
  });
  mount();
  await user.click(screen.getByRole("button", { name: "Domain review" }));
  await user.click(await screen.findByRole("button", { name: "Not now" }));
  await user.click(
    await screen.findByRole("button", { name: "Return to Review" }),
  );
  expect(await screen.findByRole("button", { name: "Not now" })).toBeTruthy();
  expect(
    writes.map(({ method, path }) => [method, path.split("/worklist")[1]]),
  ).toEqual([
    ["PUT", "/done"],
    ["POST", "/done/undo"],
  ]);
  expect(writes.map(({ body }) => body)).toEqual([
    { rows: [{ source: "domain_question", row_id: undecided.domain }] },
    { rows: [{ source: "domain_question", row_id: undecided.domain }] },
  ]);
});

it.each([
  ["Create company", "/capture/domain-questions/unreadable.example/keep"],
  ["Retry site read", "/capture/blocked-domains/unreadable.example/reopen"],
  ["Exclude domain", "/capture/domain-questions/unreadable.example/discard"],
])("%s uses its own endpoint and refreshes the review", async (label, path) => {
  const user = userEvent.setup();
  const writes: string[] = [];
  let rows = [undecided];
  transport((request, url) => {
    if (request.method === "POST") {
      writes.push(url.pathname);
      rows = [];
      return response(null, 202);
    }
    return response({ data: rows, page: { has_more: false } });
  });
  mount();
  await user.click(screen.getByRole("button", { name: "Domain review" }));
  const button = await screen.findByRole("button", { name: label });
  await waitFor(() => expect(button.hasAttribute("disabled")).toBe(false));
  await user.click(button);
  expect(await screen.findByText("No domains need review.")).toBeTruthy();
  expect(writes).toHaveLength(1);
  expect(writes[0].endsWith(path)).toBe(true);
});

it("shows read failures and retains failed decisions for retry", async () => {
  const user = userEvent.setup();
  let failedRead = true;
  transport((request) => {
    if (failedRead || request.method === "POST")
      return response(
        {
          title: "Conflict",
          status: 409,
          detail: "Review evidence has changed.",
          code: "conflict",
        },
        409,
      );
    return response({ data: [undecided], page: { has_more: false } });
  });
  mount();
  await user.click(screen.getByRole("button", { name: "Domain review" }));
  expect(await screen.findByText("Review evidence has changed.")).toBeTruthy();
  expect(screen.queryByText("No domains need review.")).toBeNull();
  await user.click(screen.getByRole("button", { name: "Close" }));
  failedRead = false;
  await user.click(screen.getByRole("button", { name: "Domain review" }));
  await user.click(
    await screen.findByRole("button", { name: "Exclude domain" }),
  );
  expect(await screen.findByText("Review evidence has changed.")).toBeTruthy();
  expect(screen.getByText(undecided.domain)).toBeTruthy();
});

it("allows personal deferral and exclusion without company-write permission", async () => {
  const user = userEvent.setup();
  transport(
    () => response({ data: [undecided], page: { has_more: false } }),
    false,
  );
  mount();
  await user.click(screen.getByRole("button", { name: "Domain review" }));
  expect(
    (
      await screen.findByRole("button", { name: "Create company" })
    ).hasAttribute("disabled"),
  ).toBe(true);
  expect(
    screen
      .getByRole("button", { name: "Retry site read" })
      .hasAttribute("disabled"),
  ).toBe(true);
  expect(
    screen
      .getByRole("button", { name: "Exclude domain" })
      .hasAttribute("disabled"),
  ).toBe(false);
  expect(
    screen.getByRole("button", { name: "Not now" }).hasAttribute("disabled"),
  ).toBe(false);
});

it("draws a redirected question through the row with the server's forwarded actions", async () => {
  const user = userEvent.setup();
  const reads: string[] = [];
  transport((_request, url) => {
    reads.push(url.searchParams.get("domain") ?? "");
    return response({ data: [forwarded], page: { has_more: false } });
  });
  const item: WorklistItem = {
    occurred_precision: "instant",
    id: forwarded.domain,
    source: "domain_question",
    category: "decisions",
    level: 6,
    consequence: "data_drifts",
    title: forwarded.domain,
    detail: "Review identity",
    because: [],
    actions: ["discard", "reopen"],
    domain_reason_code: "forwarded_identity",
  };
  mount(rowAnswer(item, briefAnswer).below);
  expect(screen.queryByRole("button", { name: "Create company" })).toBeNull();
  expect(
    screen.getByRole("button", { name: "Stop capturing this domain" }),
  ).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Choose company" }));
  expect(await screen.findByRole("dialog")).toBeTruthy();
  expect(await screen.findByText("Company for alias.example")).toBeTruthy();
  expect(reads).toEqual(["alias.example"]);
  await user.click(screen.getByRole("button", { name: "Close" }));
  expect(screen.queryByRole("dialog")).toBeNull();
});

const suggested: WorklistItem = {
  occurred_precision: "instant",
  id: forwarded.domain,
  source: "domain_question",
  category: "decisions",
  level: 6,
  consequence: "data_drifts",
  title: forwarded.domain,
  detail: "Review identity",
  because: [],
  actions: ["discard", "reopen"],
  domain_reason_code: "forwarded_identity",
  domain_context: {
    contacts: [
      { id: "00000000-0000-4000-8000-0000000000a1", name: "Alex Example" },
    ],
    contact_count: 2,
    hidden_senders: false,
    site_unread: false,
    suggested_company: {
      id: "00000000-0000-4000-8000-000000000001",
      name: "Example Company",
      basis: "redirect_target",
    },
  },
};

it("asks about the person and confirms the suggested company for the whole domain", async () => {
  const user = userEvent.setup();
  const writes: { path: string; body: unknown }[] = [];
  transport(async (request, url) => {
    writes.push({ path: url.pathname, body: await request.json() });
    return response(null, 202);
  });
  mount(<DomainQuestionAnswer item={suggested} />);
  expect(
    screen.getByText(
      "Which company do Alex Example and 1 other contact work for?",
    ),
  ).toBeTruthy();
  expect(screen.getByRole("link", { name: "Alex Example" })).toBeTruthy();
  expect(screen.getByRole("link", { name: "Example Company" })).toBeTruthy();
  expect(
    await screen.findByText(
      "Links alias.example and its 2 contacts to Example Company.",
    ),
  ).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Confirm company" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(
    writes[0].path.endsWith("/capture/domain-questions/alias.example/keep"),
  ).toBe(true);
  expect(writes[0].body).toEqual({
    company_id: "00000000-0000-4000-8000-000000000001",
  });
});

it("offers no suggestion it does not have and says who wrote when it cannot name them", () => {
  transport(() => response(null, 202));
  mount(
    <DomainQuestionAnswer
      item={{
        ...suggested,
        id: undecided.domain,
        actions: ["keep", "discard", "reopen"],
        domain_reason_code: "unevidenced",
        domain_context: {
          contacts: [],
          contact_count: 0,
          hidden_senders: true,
          sender_address: "someone@unreadable.example",
          site_unread: true,
        },
      }}
    />,
  );
  expect(
    screen.getByText("Which company is someone@unreadable.example from?"),
  ).toBeTruthy();
  expect(screen.getByText(/from a contact you cannot open/)).toBeTruthy();
  expect(screen.getByText(/could not read the website/)).toBeTruthy();
  expect(screen.queryByText("Suggested company:")).toBeNull();
  expect(screen.queryByRole("button", { name: "Confirm company" })).toBeNull();
  expect(screen.getByRole("button", { name: "Choose company" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "Create company" })).toBeTruthy();
});

it("keeps ordinary row decisions separate from forwarded review", async () => {
  const user = userEvent.setup();
  const writes: string[] = [];
  transport((request, url) => {
    expect(request.method).toBe("POST");
    writes.push(url.pathname);
    return response(null, 202);
  });
  const item: WorklistItem = {
    occurred_precision: "instant",
    id: undecided.domain,
    source: "domain_question",
    category: "decisions",
    level: 6,
    consequence: "data_drifts",
    title: undecided.domain,
    detail: "Review identity",
    because: [],
    actions: ["keep", "discard"],
    domain_reason_code: "unevidenced",
  };
  mount(<DomainQuestionAnswer item={item} />);
  await user.click(screen.getByRole("button", { name: "Create company" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  const discard = screen.getByRole("button", {
    name: "Stop capturing this domain",
  });
  await waitFor(() => expect(discard.hasAttribute("disabled")).toBe(false));
  await user.click(discard);
  await waitFor(() => expect(writes).toHaveLength(2));
  expect(
    writes[0].endsWith(`/capture/domain-questions/${undecided.domain}/keep`),
  ).toBe(true);
  expect(
    writes[1].endsWith(`/capture/domain-questions/${undecided.domain}/discard`),
  ).toBe(true);
});

const vendor: Question = {
  domain: "company-a.example",
  deferred: false,
  reason_code: "vendor_verdict",
  asked_at: "2026-10-01T10:00:00Z",
  note: "Classified as a mail or hosting vendor: sells managed hosting.",
};

it("answers a vendor question from the row with create company or keep as vendor", async () => {
  const user = userEvent.setup();
  const writes: string[] = [];
  transport((request, url) => {
    expect(request.method).toBe("POST");
    writes.push(url.pathname);
    return url.pathname.endsWith("/keep-vendor")
      ? new Response(null, { status: 204 })
      : response(null, 202);
  });
  const item: WorklistItem = {
    occurred_precision: "instant",
    id: vendor.domain,
    source: "domain_question",
    category: "decisions",
    level: 6,
    consequence: "data_drifts",
    title: vendor.domain,
    detail: "Review identity",
    because: [],
    actions: ["keep", "keep_as_vendor", "discard"],
    domain_reason_code: "vendor_verdict",
  };
  mount(<DomainQuestionAnswer item={item} />);
  expect(screen.getByRole("button", { name: "Choose company" })).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Keep as vendor" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  const create = screen.getByRole("button", { name: "Create company" });
  await waitFor(() => expect(create.hasAttribute("disabled")).toBe(false));
  await user.click(create);
  await waitFor(() => expect(writes).toHaveLength(2));
  expect(
    writes[0].endsWith(
      `/capture/domain-questions/${vendor.domain}/keep-vendor`,
    ),
  ).toBe(true);
  expect(
    writes[1].endsWith(`/capture/domain-questions/${vendor.domain}/keep`),
  ).toBe(true);
});

it("offers no keep as vendor on a question that is not a vendor question", () => {
  transport(() => response(null, 202));
  const item: WorklistItem = {
    occurred_precision: "instant",
    id: undecided.domain,
    source: "domain_question",
    category: "decisions",
    level: 6,
    consequence: "data_drifts",
    title: undecided.domain,
    detail: "Review identity",
    because: [],
    actions: ["keep", "discard", "reopen"],
    domain_reason_code: "unevidenced",
  };
  mount(<DomainQuestionAnswer item={item} />);
  expect(screen.queryByRole("button", { name: "Keep as vendor" })).toBeNull();
});

it.each([
  ["Create company", "/capture/domain-questions/company-a.example/keep"],
  ["Keep as vendor", "/capture/domain-questions/company-a.example/keep-vendor"],
])(
  "review answers a vendor question with %s and shows the triage's sentence",
  async (label, path) => {
    const user = userEvent.setup();
    const writes: string[] = [];
    let rows = [vendor];
    transport((request, url) => {
      if (request.method === "POST") {
        writes.push(url.pathname);
        rows = [];
        return new Response(null, { status: 204 });
      }
      return response({ data: rows, page: { has_more: false } });
    });
    mount();
    await user.click(screen.getByRole("button", { name: "Domain review" }));
    expect(await screen.findByText(vendor.note ?? "")).toBeTruthy();
    expect(
      screen.getByText(/reads as a mail or hosting provider/),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: "Retry site read" }),
    ).toBeNull();
    const button = screen.getByRole("button", { name: label });
    await waitFor(() => expect(button.hasAttribute("disabled")).toBe(false));
    await user.click(button);
    expect(await screen.findByText("No domains need review.")).toBeTruthy();
    expect(writes).toHaveLength(1);
    expect(writes[0].endsWith(path)).toBe(true);
  },
);
