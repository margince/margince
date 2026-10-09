// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
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
import { PrivacyInboxCard } from "./privacy";
import { type DataSubjectRequest, DsrDetail } from "./privacy.requests";

const NAMED: DataSubjectRequest = {
  id: "d1",
  kind: "erasure",
  subject_ref: "3fa85f64-5717-4562-b3fc-2c963f66afa6",
  subject_label: "Lena Hoffmann",
  subject_kind: "contact",
  status: "in_progress",
  due_at: "2026-08-01T00:00:00Z",
  created_at: "2026-07-01T00:00:00Z",
};

const HIDDEN: DataSubjectRequest = {
  ...NAMED,
  id: "d2",
  subject_ref: "9c1e8a10-0000-4000-8000-00000000c0de",
  subject_label: null,
  subject_kind: null,
  status: "open",
};

const EXTERNAL: DataSubjectRequest = {
  ...NAMED,
  id: "d3",
  kind: "rectify",
  subject_ref: "partner-ref-0042@acme.test",
  subject_label: null,
  status: "rejected",
};

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

type Patched = { path: string; body: unknown };

function stub(create?: () => Response, patched: Patched[] = []) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(request ? request.url : String(input), "http://t");
      const method = request?.method ?? init?.method ?? "GET";
      if (method === "PATCH") {
        const body: unknown = request
          ? await request.json()
          : JSON.parse(String(init?.body));
        patched.push({ path: url.pathname, body });
        return json(NAMED);
      }
      if (url.pathname.endsWith("/users")) {
        return json({
          data: [
            {
              id: "u-1",
              email: "anna@acme.test",
              display_name: "Anna Weber",
              status: "active",
              is_agent: false,
            },
          ],
          page: { next_cursor: null, has_more: false },
        });
      }
      if (url.pathname.endsWith("/me")) {
        return json(
          meFixture({
            roles: ["admin"],
            allow: {
              privacy_request: ["read", "update"],
              contact: ["update"],
            },
          }),
        );
      }
      if (url.pathname.endsWith("/data-subject-requests")) {
        if (method === "POST" && create) {
          return create();
        }
        return json({
          data: [NAMED, HIDDEN, EXTERNAL],
          page: { next_cursor: null, has_more: false },
        });
      }
      return json({ data: [], page: { next_cursor: null, has_more: false } });
    }),
  );
}

function render(ui: ReactNode = <PrivacyInboxCard />) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

function renderDrawer(dsr: DataSubjectRequest) {
  return render(
    <DsrDetail
      dsr={dsr}
      titleId="dsr-title"
      nowMs={Date.parse("2026-07-15T00:00:00Z")}
      onClose={() => {}}
      onFulfilErasure={() => {}}
    />,
  );
}

// The subject's value in the drawer's fact list.
async function subjectFact(): Promise<HTMLElement> {
  const term = await screen.findByText(en["privacy.subject"], {
    selector: "dt",
  });
  const value = term.nextElementSibling;
  if (!(value instanceof HTMLElement)) throw new Error("no subject value");
  return value;
}

beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the subject-request table", () => {
  it("names a resolved subject, withholds a hidden one, and keeps an external one as written", async () => {
    stub();
    render();

    expect(
      await screen.findByRole("button", { name: "Lena Hoffmann" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: en["notice.recordUnavailable"] }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "partner-ref-0042@acme.test" }),
    ).toBeInTheDocument();
    expect(screen.queryByText(HIDDEN.subject_ref)).toBeNull();
  });

  it("reads kinds and statuses in words, the filter included", async () => {
    stub();
    render();

    const row = (await screen.findByText("Lena Hoffmann")).closest("tr");
    if (!(row instanceof HTMLElement)) throw new Error("no row");
    expect(
      within(row).getByText(en["privacy.kindErasure"]),
    ).toBeInTheDocument();
    expect(
      within(row).getByText(en["privacy.statusInProgress"]),
    ).toBeInTheDocument();
    const filter = screen.getByRole("group", {
      name: en["privacy.facetLabel"],
    });
    expect(
      within(filter).getByRole("button", { name: en["privacy.statusOpen"] }),
    ).toBeInTheDocument();
    expect(within(filter).queryByText("in progress")).toBeNull();
  });

  it("puts a refused due date beside its field", async () => {
    stub(() =>
      json(
        {
          title: "Unprocessable Entity",
          status: 422,
          code: "validation_error",
          details: {
            errors: [
              { field: "due_at", code: "required", message: "required" },
            ],
          },
        },
        422,
      ),
    );
    render();

    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: en["privacy.newRequest"] }),
    );
    const dialog = await screen.findByRole("dialog");
    await pickOption(
      user,
      within(dialog).getByLabelText(en["privacy.kind"]),
      en["privacy.kindAccess"],
    );
    await user.type(
      within(dialog).getByLabelText(en["privacy.subjectRef"]),
      "anna@acme.test",
    );
    await user.type(within(dialog).getByLabelText(/^Due/), "2026-09-30");
    await user.click(
      within(dialog).getByRole("button", { name: en["privacy.openRequest"] }),
    );

    const due = within(dialog).getByLabelText(/^Due/);
    expect(
      await within(dialog).findByText(en["privacy.dueRequired"]),
    ).toBeInTheDocument();
    expect(due).toHaveAttribute("aria-invalid", "true");
  });
});

describe("the subject-request drawer", () => {
  it("links a contact subject to the contact", async () => {
    stub();
    renderDrawer(NAMED);

    const link = within(await subjectFact()).getByRole("link", {
      name: "Lena Hoffmann",
    });
    expect(link).toHaveAttribute("href", `#/contacts/${NAMED.subject_ref}`);
  });

  it("links a lead subject to the lead, never to a contact of the same id", async () => {
    stub();
    renderDrawer({
      ...NAMED,
      subject_label: "Jonas Berg",
      subject_kind: "lead",
    });

    const link = within(await subjectFact()).getByRole("link", {
      name: "Jonas Berg",
    });
    expect(link).toHaveAttribute("href", `#/leads/${NAMED.subject_ref}`);
  });

  it("names an unresolved subject neutrally and links nowhere", async () => {
    stub();
    renderDrawer(HIDDEN);

    const value = await subjectFact();
    expect(value).toHaveTextContent(en["notice.recordUnavailable"]);
    expect(within(value).queryByRole("link")).toBeNull();
    expect(screen.queryByText(HIDDEN.subject_ref)).toBeNull();
  });

  it("hands a request back to nobody as a null assignee", async () => {
    const patched: Patched[] = [];
    stub(undefined, patched);
    renderDrawer({ ...NAMED, assignee_id: "u-1" });

    const user = userEvent.setup();
    const picker = await screen.findByRole("combobox", {
      name: en["privacy.assignee"],
    });
    await within(picker).findByText("Anna Weber");
    await pickOption(user, picker, en["notice.unassigned"]);

    await waitFor(() => expect(patched).toHaveLength(1));
    expect(patched[0]).toEqual({
      path: `/v1/data-subject-requests/${NAMED.id}`,
      body: { assignee_id: null },
    });
  });
});
