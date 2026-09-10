/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { EditContactMethodsModal } from "./personcontactedit";

type Person = components["schemas"]["Person"];

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

// Every PATCH this modal sends, in order, so a test can assert what was SENT
// rather than what the component happened to render afterwards — the same
// discipline personrail.test.tsx holds for the employment modal's requests.
// `ifMatch` rides along the same way automations.test.tsx captures it: the
// version-pinning tests need to see the precondition header, not just the body.
const sent: Array<{
  method: string;
  path: string;
  body: unknown;
  ifMatch: string | null;
}> = [];

function mountFetchRecorder() {
  sent.length = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      // openapi-fetch hands `fetch` a Request, not (url, init), so the body is
      // on the Request and reading it needs a clone — consuming the original
      // would leave the real call with an empty body.
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test",
      );
      const method = request?.method ?? init?.method ?? "GET";
      if (method === "PATCH") {
        const raw = request
          ? await request.clone().text()
          : String(init?.body ?? "");
        const body: unknown = raw === "" ? {} : JSON.parse(raw);
        const ifMatch = request?.headers.get("If-Match") ?? null;
        sent.push({ method, path: url.pathname, body, ifMatch });
        return json(body);
      }
      return json({ data: [], page: { has_more: false, next_cursor: null } });
    }),
  );
  return sent;
}

function renderModal({
  open,
  person,
  onClose,
}: Readonly<{
  open: boolean;
  person: Person;
  onClose?: () => void;
}>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const view = render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <EditContactMethodsModal
          open={open}
          onClose={onClose ?? (() => {})}
          person={person}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { ...view, client };
}

// Narrowed, not asserted: the recorded body came off the wire as `unknown`,
// and casting it with `as` would be exactly the unchecked claim this suite
// exists to catch elsewhere in the tree.
function asRecord(value: unknown, label: string): Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new Error(`${label} was not a JSON object: ${JSON.stringify(value)}`);
  }
  return { ...value };
}

function asArray(value: unknown, label: string): unknown[] {
  if (!Array.isArray(value)) {
    throw new Error(`${label} was not a JSON array: ${JSON.stringify(value)}`);
  }
  return value;
}

async function findPatch(): Promise<{
  method: string;
  path: string;
  body: unknown;
  ifMatch: string | null;
}> {
  await waitFor(() => {
    if (!sent.some((request) => request.method === "PATCH")) {
      throw new Error(
        `no PATCH was sent; requests were ${JSON.stringify(sent)}`,
      );
    }
  });
  const patch = sent.find((request) => request.method === "PATCH");
  if (!patch) {
    throw new Error("a PATCH was reported present but not found");
  }
  return patch;
}

function person(overrides: Partial<Person>): Person {
  return {
    id: "p-1",
    full_name: "Dana",
    source: "manual",
    captured_by: "human:u-1",
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-08-01T08:00:00Z",
    version: 4,
    writable: true,
    ...overrides,
  };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("editing a person's contact methods", () => {
  it("sends one whole-list PATCH with the corrected address and the kept number", async () => {
    const user = userEvent.setup();
    mountFetchRecorder();
    const dana = person({
      emails: [
        {
          id: "e-1",
          email: "old@acme.com",
          email_type: "work",
          is_primary: true,
          position: 0,
          source: "manual",
          captured_by: "human:u-1",
        },
      ],
      phones: [
        {
          id: "ph-1",
          phone: "+49301234",
          phone_type: "work",
          is_primary: true,
          position: 0,
          source: "manual",
          captured_by: "human:u-1",
        },
      ],
    });
    renderModal({ open: true, person: dana });

    const value = screen.getByDisplayValue("old@acme.com");
    await user.clear(value);
    await user.type(value, "new@acme.com");
    await user.click(screen.getByRole("button", { name: /save/i }));

    const patch = await findPatch();
    expect(patch.path.endsWith("/people/p-1")).toBe(true);
    const body = asRecord(patch.body, "the contact-methods patch");
    const emails = asArray(body.emails, "the patched emails");
    const phones = asArray(body.phones, "the patched phones");
    expect(asRecord(emails[0], "the first email").email).toBe("new@acme.com");
    // The untouched number rides along — a whole-list PATCH that dropped it
    // would delete a phone nobody asked to remove.
    expect(asRecord(phones[0], "the first phone").phone).toBe("+49301234");
  });

  it("removes an address and sends the survivor alone", async () => {
    const user = userEvent.setup();
    mountFetchRecorder();
    const dana = person({
      emails: [
        {
          id: "e-1",
          email: "first@acme.com",
          email_type: "work",
          is_primary: true,
          position: 0,
          source: "manual",
          captured_by: "human:u-1",
        },
        {
          id: "e-2",
          email: "second@acme.com",
          email_type: "personal",
          is_primary: false,
          position: 1,
          source: "manual",
          captured_by: "human:u-1",
        },
      ],
      phones: [],
    });
    renderModal({ open: true, person: dana });

    await user.click(
      screen.getByRole("button", { name: /remove.*first@acme\.com/i }),
    );
    await user.click(screen.getByRole("button", { name: /save/i }));

    const patch = await findPatch();
    const body = asRecord(patch.body, "the contact-methods patch");
    const emails = asArray(body.emails, "the patched emails");
    expect(emails).toHaveLength(1);
    expect(asRecord(emails[0], "the surviving email").email).toBe(
      "second@acme.com",
    );
  });

  it("drops blank rows when saving contact methods", async () => {
    const user = userEvent.setup();
    mountFetchRecorder();
    const dana = person({
      emails: [
        {
          id: "e-1",
          email: "existing@acme.com",
          email_type: "work",
          is_primary: true,
          position: 0,
          source: "manual",
          captured_by: "human:u-1",
        },
      ],
      phones: [
        {
          id: "ph-1",
          phone: "+49301234",
          phone_type: "work",
          is_primary: true,
          position: 0,
          source: "manual",
          captured_by: "human:u-1",
        },
      ],
    });
    renderModal({ open: true, person: dana });

    // Append a blank email row and leave it empty
    await user.click(screen.getByRole("button", { name: /add email/i }));
    // Append a blank phone row and leave it empty
    await user.click(screen.getByRole("button", { name: /add phone/i }));
    await user.click(screen.getByRole("button", { name: /save/i }));

    const patch = await findPatch();
    const body = asRecord(patch.body, "the contact-methods patch");
    const emails = asArray(body.emails, "the patched emails");
    const phones = asArray(body.phones, "the patched phones");

    // Blank rows must be dropped, so only the existing email should remain
    expect(emails).toHaveLength(1);
    const emailRecord = asRecord(emails[0], "the first email");
    expect(emailRecord.email).toBe("existing@acme.com");
    expect(emailRecord.is_primary).toBe(true);
    expect(emailRecord.position).toBe(0);

    // Same for phones: only the existing phone should remain
    expect(phones).toHaveLength(1);
    const phoneRecord = asRecord(phones[0], "the first phone");
    expect(phoneRecord.phone).toBe("+49301234");
    expect(phoneRecord.is_primary).toBe(true);
    expect(phoneRecord.position).toBe(0);
  });

  it("reorders emails and saves the new position", async () => {
    const user = userEvent.setup();
    mountFetchRecorder();
    const dana = person({
      emails: [
        {
          id: "e-1",
          email: "a@acme.com",
          email_type: "work",
          is_primary: true,
          position: 0,
          source: "manual",
          captured_by: "human:u-1",
        },
        {
          id: "e-2",
          email: "b@acme.com",
          email_type: "personal",
          is_primary: false,
          position: 1,
          source: "manual",
          captured_by: "human:u-1",
        },
      ],
      phones: [],
    });
    renderModal({ open: true, person: dana });

    // Move the FIRST row (a@acme.com) down, so the saved order becomes b, a.
    await user.click(
      screen.getByRole("button", { name: /move down.*a@acme\.com/i }),
    );
    await user.click(screen.getByRole("button", { name: /save/i }));

    const patch = await findPatch();
    const body = asRecord(patch.body, "the contact-methods patch");
    const emails = asArray(body.emails, "the patched emails");
    expect(emails).toHaveLength(2);
    const first = asRecord(emails[0], "the first email");
    const second = asRecord(emails[1], "the second email");
    expect(first.email).toBe("b@acme.com");
    expect(first.position).toBe(0);
    expect(second.email).toBe("a@acme.com");
    expect(second.position).toBe(1);
  });

  it("reconciles the primary email when the primary row's type changes to match another primary", async () => {
    const user = userEvent.setup();
    mountFetchRecorder();
    const dana = person({
      emails: [
        {
          id: "e-1",
          email: "a@acme.com",
          email_type: "work",
          is_primary: true,
          position: 0,
          source: "manual",
          captured_by: "human:u-1",
        },
        {
          id: "e-2",
          email: "b@acme.com",
          email_type: "personal",
          is_primary: true,
          position: 1,
          source: "manual",
          captured_by: "human:u-1",
        },
      ],
      phones: [],
    });
    renderModal({ open: true, person: dana });

    // Both rows arrived primary within their OWN type (work, personal) — a
    // state the seed data can hold but the UI itself cannot produce. Switch
    // b@acme.com's type to "work", the type a@acme.com already occupies.
    const typeSelects = screen.getAllByRole("combobox", { name: "Type" });
    await user.click(typeSelects[1]);
    await user.click(screen.getByRole("option", { name: "Work" }));
    await user.click(screen.getByRole("button", { name: /save/i }));

    const patch = await findPatch();
    const body = asRecord(patch.body, "the contact-methods patch");
    const emails = asArray(body.emails, "the patched emails");
    const workEmails = emails
      .map((row) => asRecord(row, "an email row"))
      .filter((row) => row.email_type === "work");
    expect(workEmails).toHaveLength(2);
    const primaryWorkEmails = workEmails.filter((row) => row.is_primary);
    // b@acme.com is the row whose type changed and which carried is_primary
    // into the new type, so it stays primary and a@acme.com is demoted.
    expect(primaryWorkEmails).toHaveLength(1);
    expect(primaryWorkEmails[0]?.email).toBe("b@acme.com");
  });

  it("reconciles the primary phone when the primary row's type changes to match another primary", async () => {
    const user = userEvent.setup();
    mountFetchRecorder();
    const dana = person({
      emails: [],
      phones: [
        {
          id: "ph-1",
          phone: "+49301111",
          phone_type: "work",
          is_primary: true,
          position: 0,
          source: "manual",
          captured_by: "human:u-1",
        },
        {
          id: "ph-2",
          phone: "+49302222",
          phone_type: "mobile",
          is_primary: true,
          position: 1,
          source: "manual",
          captured_by: "human:u-1",
        },
      ],
    });
    renderModal({ open: true, person: dana });

    const typeSelects = screen.getAllByRole("combobox", { name: "Type" });
    await user.click(typeSelects[1]);
    await user.click(screen.getByRole("option", { name: "Work" }));
    await user.click(screen.getByRole("button", { name: /save/i }));

    const patch = await findPatch();
    const body = asRecord(patch.body, "the contact-methods patch");
    const phones = asArray(body.phones, "the patched phones");
    const workPhones = phones
      .map((row) => asRecord(row, "a phone row"))
      .filter((row) => row.phone_type === "work");
    expect(workPhones).toHaveLength(2);
    const primaryWorkPhones = workPhones.filter((row) => row.is_primary);
    expect(primaryWorkPhones).toHaveLength(1);
    expect(primaryWorkPhones[0]?.phone).toBe("+49302222");
  });

  it("pins the save to the version open when the modal opened, not a later refetch", async () => {
    const user = userEvent.setup();
    mountFetchRecorder();
    const dana = person({
      version: 4,
      emails: [
        {
          id: "e-1",
          email: "old@acme.com",
          email_type: "work",
          is_primary: true,
          position: 0,
          source: "manual",
          captured_by: "human:u-1",
        },
      ],
      phones: [],
    });
    const { rerender, client } = renderModal({ open: true, person: dana });

    // A background person360 refetch bumps the `person` prop's version while
    // the modal stays open (no close/reopen) — the staged edits were made
    // against version 4 and the save must still pin to that, not the newer
    // version that arrived underneath the open modal.
    rerender(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <EditContactMethodsModal
            open={true}
            onClose={() => {}}
            person={{ ...dana, version: 5 }}
          />
        </LocaleProvider>
      </QueryClientProvider>,
    );

    await user.click(screen.getByRole("button", { name: /save/i }));

    const patch = await findPatch();
    expect(patch.ifMatch).toBe("4");
  });
});
