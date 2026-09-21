// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { pickOption } from "../design-system/select-testing";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { NoticeCasesCard } from "./noticecases";

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

// One owed duty, one already excused — enough to tell the two facets apart and
// to prove a closed case offers no actions.
const OWED = {
  id: "case-1",
  contact_id: "contact-1",
  rule: "art14",
  due_at: "2026-01-01T00:00:00Z",
  state: "open",
  attempts: 0,
  created_at: "2025-12-01T00:00:00Z",
};

const EXCUSED = {
  id: "case-2",
  contact_id: "contact-2",
  rule: "art14",
  due_at: "2026-01-01T00:00:00Z",
  state: "exempt_with_reason",
  attempts: 0,
  created_at: "2025-12-01T00:00:00Z",
  resolution_note: "Told at the trade fair on the 3rd",
  completed_at: "2026-02-01T00:00:00Z",
};

type Sent = { key: string; url: string; body: unknown };

/**
 * One page of the queue, as the route answers it.
 *
 * `page` is required on the wire — the queue is keyset-paged because an
 * installation owes as many duties as it owes — so a fixture without one is a
 * payload no server can send, and the screen's walk reads it.
 */
function queuePage(data: unknown[]) {
  return jsonResponse({ data, page: { next_cursor: null, has_more: false } });
}

function stubRoutes(
  overrides: Record<string, () => Response> = {},
  sent: Sent[] = [],
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      let body: unknown = null;
      if (method !== "GET") {
        try {
          body = request
            ? await request.json()
            : JSON.parse(String(init?.body));
        } catch {
          body = null;
        }
      }
      sent.push({ key, url: url.pathname + url.search, body });
      const override = overrides[key];
      if (override) return override();
      if (key === "GET /privacy/notice-cases") {
        return queuePage([OWED]);
      }
      if (key === "GET /users") {
        return jsonResponse({
          data: [{ id: "user-1", email: "o@x.test", display_name: "Officer" }],
          page: { next_cursor: null, has_more: false },
        });
      }
      if (key === "GET /me") {
        return jsonResponse(
          meFixture({
            roles: ["admin"],
            allow: { privacy_request: ["read", "update"] },
          }),
        );
      }
      return jsonResponse({});
    }),
  );
  return sent;
}

beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));
afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
});

describe("the disclosure-duty queue", () => {
  it("asks the server for the owed states, not for everything", async () => {
    // The facet is a server-side filter. A client re-slice would hide rows the
    // server never told us about, and the count on screen would disagree with
    // the one the queue has.
    const sent = stubRoutes();
    render(<NoticeCasesCard />);

    await screen.findByTestId("notice-case-case-1");
    const listed = sent.find((s) => s.key === "GET /privacy/notice-cases");
    expect(listed?.url).toContain("state=open");
    expect(listed?.url).toContain("state=assigned");
    expect(listed?.url).toContain("state=queued");
    expect(listed?.url).toContain("state=blocked");
    // The four that END a duty are not asked for on the owed facet.
    expect(listed?.url).not.toContain("state=completed");
    expect(listed?.url).not.toContain("state=exempt_with_reason");
  });

  it("marks a duty whose deadline has passed", async () => {
    stubRoutes();
    render(<NoticeCasesCard />);

    const row = await screen.findByTestId("notice-case-case-1");
    expect(within(row).getByText(en["notice.overdue"])).toBeInTheDocument();
  });

  it("says a duty nobody has taken is unclaimed", async () => {
    stubRoutes();
    render(<NoticeCasesCard />);

    const row = await screen.findByTestId("notice-case-case-1");
    expect(within(row).getByText(en["notice.unclaimed"])).toBeInTheDocument();
  });

  it("offers no actions on a duty that has already ended", async () => {
    // The server refuses both, so offering them would be a button that fails.
    stubRoutes({
      "GET /privacy/notice-cases": () => queuePage([EXCUSED]),
    });
    render(<NoticeCasesCard />);

    const row = await screen.findByTestId("notice-case-case-2");
    expect(
      within(row).queryByRole("button", { name: en["notice.excuse"] }),
    ).not.toBeInTheDocument();
    // And it still shows the ground, which is what an auditor opens it for.
    expect(within(row).getByText(/Told at the trade fair/)).toBeInTheDocument();
  });

  it("sends the ground with an excuse, and the state the officer chose", async () => {
    const sent = stubRoutes({
      "POST /privacy/notice-cases/case-1/excuse": () =>
        jsonResponse({ ...OWED, state: "exempt_with_reason" }),
    });
    render(<NoticeCasesCard />);

    const row = await screen.findByTestId("notice-case-case-1");
    const user = userEvent.setup();
    await user.click(
      within(row).getByRole("button", { name: en["notice.excuse"] }),
    );

    const dialog = await screen.findByRole("dialog");
    await pickOption(
      user,
      within(dialog).getByRole("combobox"),
      en["notice.excuseExempt"],
    );
    await user.type(
      within(dialog).getByRole("textbox"),
      "Art. 14(5)(b): no address was ever held",
    );
    await user.click(
      within(dialog).getByRole("button", { name: en["notice.excuseConfirm"] }),
    );

    await waitFor(() => {
      const write = sent.find((s) => s.key.startsWith("POST /privacy/notice"));
      expect(write?.body).toEqual({
        state: "exempt_with_reason",
        resolution_note: "Art. 14(5)(b): no address was ever held",
      });
    });
  });

  it("will not submit an excuse with no ground", async () => {
    // The whole reason these two states exist beside not_required is that they
    // keep the reason. A blank one would close a compliance record with
    // nothing written on it, and the server refuses it.
    stubRoutes();
    render(<NoticeCasesCard />);

    const row = await screen.findByTestId("notice-case-case-1");
    const user = userEvent.setup();
    await user.click(
      within(row).getByRole("button", { name: en["notice.excuse"] }),
    );

    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByRole("button", { name: en["notice.excuseConfirm"] }),
    ).toBeDisabled();
  });

  it("asks for every state on the All facet, because the server's default is owed", async () => {
    // The defect this holds: omitting the state filter makes the server answer
    // with the unresolved states, so "All" returned exactly the same rows as
    // "Still owed" and every closed case was invisible. Nothing failed — the
    // facet simply lied.
    const sent = stubRoutes();
    render(<NoticeCasesCard />);

    await screen.findByTestId("notice-case-case-1");
    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", { name: en["notice.facetAll"] }),
    );

    await waitFor(() => {
      const reads = sent.filter((s) => s.key === "GET /privacy/notice-cases");
      const last = reads[reads.length - 1];
      expect(last?.url).toContain("state=completed");
      expect(last?.url).toContain("state=exempt_with_reason");
      expect(last?.url).toContain("state=not_required");
      expect(last?.url).toContain("state=provided_elsewhere");
    });
  });

  it("starts a second duty's excuse form clean after a successful one", async () => {
    // The modal wrapper holds its own useState and stays mounted whether or
    // not the dialog is showing, so a change of subject does not by itself
    // reset the form. Dismissing clears the ground explicitly; SUCCEEDING did
    // not, and neither path ever reset the chosen kind — so an officer who
    // excused case A would open case B with A's words and A's claim already
    // filled in, one keystroke from confirming them.
    stubRoutes({
      "GET /privacy/notice-cases": () =>
        queuePage([OWED, { ...OWED, id: "case-3" }]),
      "POST /privacy/notice-cases/case-1/excuse": () =>
        jsonResponse({ ...OWED, state: "exempt_with_reason" }),
    });
    render(<NoticeCasesCard />);

    const user = userEvent.setup();
    const first = await screen.findByTestId("notice-case-case-1");
    await user.click(
      within(first).getByRole("button", { name: en["notice.excuse"] }),
    );
    let dialog = await screen.findByRole("dialog");
    await pickOption(
      user,
      within(dialog).getByRole("combobox"),
      en["notice.excuseExempt"],
    );
    await user.type(within(dialog).getByRole("textbox"), "A's ground");
    await user.click(
      within(dialog).getByRole("button", { name: en["notice.excuseConfirm"] }),
    );
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );

    const second = await screen.findByTestId("notice-case-case-3");
    await user.click(
      within(second).getByRole("button", { name: en["notice.excuse"] }),
    );
    dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByRole("textbox")).toHaveValue("");
    // And the claim resets too, to the one a reader has not asserted anything
    // by leaving alone.
    expect(within(dialog).getByRole("combobox")).toHaveTextContent(
      en["notice.excuseProvided"],
    );
  });

  it("says why it is empty rather than showing nothing", async () => {
    stubRoutes({
      "GET /privacy/notice-cases": () => queuePage([]),
    });
    render(<NoticeCasesCard />);

    expect(await screen.findByText(en["notice.emptyOwed"])).toBeInTheDocument();
  });

  it("withholds the queue from a seat without the privacy grant", async () => {
    // Withheld, not absent. An absent card would read as "no duties", which is
    // a different claim entirely and the wrong one to make here.
    stubRoutes({
      "GET /me": () =>
        jsonResponse(
          meFixture({ roles: ["rep"], allow: { contact: ["read"] } }),
        ),
    });
    render(<NoticeCasesCard />);

    expect(
      await screen.findByText(en["notice.readOnlyForPrivacy"]),
    ).toBeInTheDocument();
    expect(screen.queryByTestId("notice-case-case-1")).not.toBeInTheDocument();
  });

  it("reaches a duty the first page did not carry", async () => {
    // The queue is as long as the installation's obligations are. Before this
    // the screen asked for one bounded page and offered nothing further, so an
    // officer who worked what they were shown would believe they had seen every
    // duty owed — the tail was not slow to reach, it was absent from every
    // answer the route could give.
    const SECOND = {
      ...OWED,
      id: "case-9",
      contact_id: "contact-9",
      due_at: "2026-03-01T00:00:00Z",
    };
    const sent: Sent[] = [];
    stubRoutes(
      {
        "GET /privacy/notice-cases": () => {
          const asked = sent.filter(
            (s) => s.key === "GET /privacy/notice-cases",
          );
          const first = asked.length === 1;
          return jsonResponse({
            data: [first ? OWED : SECOND],
            page: first
              ? { next_cursor: "page-2", has_more: true }
              : { next_cursor: null, has_more: false },
          });
        },
      },
      sent,
    );
    render(<NoticeCasesCard />);

    await screen.findByTestId("notice-case-case-1");
    expect(screen.queryByTestId("notice-case-case-9")).not.toBeInTheDocument();

    await userEvent.click(
      await screen.findByRole("button", { name: en["list.loadMore"] }),
    );

    // Both duties on screen at once: a walk, not a replacement. An officer
    // working down the queue must not lose the row they were reading.
    await screen.findByTestId("notice-case-case-9");
    expect(screen.getByTestId("notice-case-case-1")).toBeInTheDocument();
    // And the second request carried the cursor the first page handed back,
    // rather than re-asking for the same page forever.
    const asked = sent.filter((s) => s.key === "GET /privacy/notice-cases");
    expect(asked).toHaveLength(2);
    expect(asked[1].url).toContain("cursor=page-2");
  });
});
