// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { pickOption } from "../design-system/select-testing";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { PrivacyInboxCard } from "./privacy";

type DataSubjectRequest = components["schemas"]["DataSubjectRequest"];

const NAMED: DataSubjectRequest = {
  id: "d1",
  kind: "erasure",
  subject_ref: "3fa85f64-5717-4562-b3fc-2c963f66afa6",
  subject_label: "Lena Hoffmann",
  status: "in_progress",
  due_at: "2026-08-01T00:00:00Z",
  created_at: "2026-07-01T00:00:00Z",
};

const HIDDEN: DataSubjectRequest = {
  ...NAMED,
  id: "d2",
  subject_ref: "9c1e8a10-0000-4000-8000-00000000c0de",
  subject_label: null,
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

function stub(create?: () => Response) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(request ? request.url : String(input), "http://t");
      const method = request?.method ?? init?.method ?? "GET";
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

function render() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <PrivacyInboxCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
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
      screen.getByRole("button", { name: en["notice.contactHidden"] }),
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
