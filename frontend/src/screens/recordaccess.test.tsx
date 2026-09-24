/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { components } from "../api/schema";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { en } from "../i18n/en";
import { useContact360 } from "./contact360";
import { RecordAccess } from "./recordaccess";

type Contact = components["schemas"]["Contact"];
type Company = components["schemas"]["Company"];

const base: Contact = {
  id: "p-1",
  full_name: "Dana Buyer",
  source: "gmail:seed",
  captured_by: "connector:gmail",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
  // The write pins the row it overwrites, so a fixture with no version is a
  // row this panel refuses to write — see the last test in this file.
  version: 7,
};

const seatMayWrite = {
  user: { id: "u1", email: "rep@example.test", full_name: "A Rep" },
  authorization: {
    seat_type: "full",
    objects: { contact: { read: true, update: true } },
  },
};

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

function stub(
  sent: string[] = [],
  writes: { body: unknown; version: string | null }[] = [],
  status = 200,
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
      sent.push(key);
      if (key === "GET /me") return json(seatMayWrite);
      if (method === "PATCH" && request) {
        writes.push({
          body: await request.json(),
          version: request.headers.get("If-Match"),
        });
        if (status !== 200) return json({ status, title: "Conflict" }, status);
      }
      return json({});
    }),
  );
}

function LiveRecordAccess() {
  const view = useContact360(base.id);
  return view.data ? (
    <RecordAccess kind="contact" record={view.data.contact} />
  ) : null;
}

function draw(contact?: Contact) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        {contact ? (
          <RecordAccess kind="contact" record={contact} />
        ) : (
          <LiveRecordAccess />
        )}
        <ToastRegion />
      </ToastProvider>
    </QueryClientProvider>,
  );
}

beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("RecordAccess — a contact", () => {
  it("shows the private visibility badge beside its action", async () => {
    stub();
    draw({ ...base, visibility: "owner", writable: true, owner_id: "u1" });
    expect(await screen.findByText("Only you")).toBeTruthy();
  });

  it("says a promoted contact is the company's", async () => {
    stub();
    draw({ ...base, visibility: "workspace", writable: true });
    // "Shared" and not "Team": the column has no team value, and the tooltip
    // beside this badge says everyone in the company can see the record.
    expect(await screen.findByText("Shared")).toBeTruthy();
  });

  it("publishes a private contact through the ordinary contact patch", async () => {
    const user = userEvent.setup();
    const sent: string[] = [];
    const writes: { body: unknown; version: string | null }[] = [];
    stub(sent, writes);
    draw({ ...base, visibility: "owner", writable: true, owner_id: "u1" });
    await user.click(
      await screen.findByRole("button", {
        name: /share with the company/i,
      }),
    );
    expect(sent).toContain("PATCH /contacts/p-1");
    expect(writes).toEqual([
      { body: { visibility: "workspace" }, version: "7" },
    ]);
  });

  it("makes a workspace contact private again — the direction that did not exist", async () => {
    const user = userEvent.setup();
    // The whole point of the change. A contact the sender classifier published
    // with nobody approving it could not be narrowed by anybody, its own owner
    // included.
    const sent: string[] = [];
    stub(sent);
    draw({ ...base, visibility: "workspace", writable: true, owner_id: "u1" });
    await user.click(
      await screen.findByRole("button", { name: /make private/i }),
    );
    expect(sent).toContain("PATCH /contacts/p-1");
  });

  it("offers no verb to a reader who may not write the record", async () => {
    stub();
    draw({
      ...base,
      visibility: "owner",
      writable: false,
      owner_id: "someone-else",
    });
    expect(await screen.findByText("Only you")).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: /share with the company/i }),
    ).toBeNull();
  });

  it("offers the verb to a colleague holding a write grant", async () => {
    const user = userEvent.setup();
    // Write access is the whole gate now. The patch path runs the ordinary
    // write test — object grant plus EnsureWritable — so a grant holder the
    // server would admit must not be refused by the drawing.
    const sent: string[] = [];
    stub(sent);
    draw({
      ...base,
      visibility: "owner",
      writable: true,
      owner_id: "someone-else",
    });
    await user.click(
      await screen.findByRole("button", {
        name: /share with the company/i,
      }),
    );
    expect(sent).toContain("PATCH /contacts/p-1");
  });

  it("offers no verb on any contact to a reader who cannot write it", async () => {
    stub();
    draw({ ...base, visibility: "workspace", writable: false, owner_id: "u1" });
    expect(await screen.findByText("Shared")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /make private/i })).toBeNull();
  });

  it("refuses to write a row it read back without a version", async () => {
    const user = userEvent.setup();
    // Unpinned is last-write-wins, and this write moves the column that
    // decides who may read the record. The refusal surfaces through the
    // mutation's error path rather than sending an unconditional PATCH.
    const sent: string[] = [];
    stub(sent);
    draw({
      ...base,
      version: undefined,
      visibility: "workspace",
      writable: true,
      owner_id: "u1",
    });
    await user.click(
      await screen.findByRole("button", { name: /make private/i }),
    );
    expect(sent).not.toContain("PATCH /contacts/p-1");
  });

  it("keeps archived visibility readable without a toggle", () => {
    stub();
    draw({
      ...base,
      visibility: "workspace",
      writable: true,
      archived_at: "2026-08-02T00:00:00Z",
    });
    expect(screen.getByText("Shared")).toBeTruthy();
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("keeps the private badge and explains a refused sharing attempt", async () => {
    const user = userEvent.setup();
    stub([], [], 409);
    draw({ ...base, visibility: "owner", writable: true, owner_id: "u1" });
    await user.tab();
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: /share with the company/i }),
    );
    await user.keyboard("{Enter}");
    expect(await screen.findByRole("alert")).toBeTruthy();
    expect(screen.getByText("Only you")).toBeTruthy();
    expect(
      screen.queryByText("This contact is now visible to all users."),
    ).toBeNull();
  });

  it("refreshes a stale version, then shares and makes private again", async () => {
    const user = userEvent.setup();
    let version = 7;
    let visibility: "owner" | "workspace" = "owner";
    const writes: { version: string | null; body: unknown }[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (!(input instanceof Request))
          throw new Error("Expected an API request");
        if (input.method === "PATCH") {
          const body: unknown = await input.json();
          writes.push({ version: input.headers.get("If-Match"), body });
          version++;
          if (writes.length === 1)
            return json({ status: 409, code: "version_skew" }, 409);
          if (
            typeof body === "object" &&
            body !== null &&
            "visibility" in body &&
            (body.visibility === "owner" || body.visibility === "workspace")
          ) {
            visibility = body.visibility;
          }
          return json({});
        }
        return json({
          contact: {
            ...base,
            version,
            visibility,
            writable: true,
            owner_id: "u1",
          },
        });
      }),
    );
    draw();
    await user.click(
      await screen.findByRole("button", { name: /share with the company/i }),
    );
    expect((await screen.findByRole("alert")).textContent).toBe(
      en["edit.versionSkew"],
    );
    await user.click(
      screen.getByRole("button", { name: /share with the company/i }),
    );
    await user.click(
      await screen.findByRole("button", { name: /make private/i }),
    );
    await screen.findByText("Only you");
    expect(writes).toEqual([
      { version: "7", body: { visibility: "workspace" } },
      { version: "8", body: { visibility: "workspace" } },
      { version: "9", body: { visibility: "owner" } },
    ]);
  });

  it("draws nothing at all when the server sent no visibility", () => {
    // A server too old to send it, or a path that does not. Guessing
    // `workspace` would tell a reader their private contact is public.
    stub();
    const { container } = draw({ ...base, writable: true });
    expect(container.innerHTML).toBe("");
  });
});

// The company half. The same component, so the cases that are about the
// COMPONENT are not repeated here — these are the ones that would pass on a
// contact and still ship a broken company: the endpoint it writes to, the
// account noun, and the account's own private state.
describe("RecordAccess — a company", () => {
  const company: Company = {
    id: "c-1",
    display_name: "Weber GmbH",
    source: "gmail:seed",
    captured_by: "connector:gmail",
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-08-01T08:00:00Z",
    version: 4,
  };

  function drawCompany(record: Company) {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    return render(
      <QueryClientProvider client={client}>
        <ToastProvider>
          <RecordAccess kind="company" record={record} />
          <ToastRegion />
        </ToastProvider>
      </QueryClientProvider>,
    );
  }

  it("says a capture-private account is the owner's", async () => {
    stub();
    drawCompany({
      ...company,
      visibility: "owner",
      writable: true,
      owner_id: "u1",
    });
    expect(await screen.findByText("Only you")).toBeTruthy();
  });

  it("publishes a private account through the company patch", async () => {
    const user = userEvent.setup();
    // The door the product did not have. A company capture minted owner-scoped
    // could only be widened by a sender verdict, so an account the classifier
    // never asked about stayed private with nothing a human could press.
    const sent: string[] = [];
    const writes: { body: unknown; version: string | null }[] = [];
    stub(sent, writes);
    drawCompany({
      ...company,
      visibility: "owner",
      writable: true,
      owner_id: "u1",
    });
    await user.click(
      await screen.findByRole("button", { name: /share with the company/i }),
    );
    // The COMPANY endpoint: a component that wrote /contacts/c-1 would pass
    // every other assertion in this block.
    expect(sent).toContain("PATCH /companies/c-1");
    expect(writes).toEqual([
      { body: { visibility: "workspace" }, version: "4" },
    ]);
  });

  it("makes a shared account private again", async () => {
    const user = userEvent.setup();
    const sent: string[] = [];
    stub(sent);
    drawCompany({
      ...company,
      visibility: "workspace",
      writable: true,
      owner_id: "u1",
    });
    await user.click(
      await screen.findByRole("button", { name: /make private/i }),
    );
    expect(sent).toContain("PATCH /companies/c-1");
  });

  it("names the account rather than the contact in its own sentence", async () => {
    stub();
    drawCompany({
      ...company,
      visibility: "workspace",
      writable: true,
      owner_id: "u1",
    });
    // The region's accessible name, which is the sentence a screen reader
    // reads first. Sharing one string with the contact header would call a
    // company a contact.
    expect(
      await screen.findByRole("region", {
        name: en["recordAccess.company.title"],
      }),
    ).toBeTruthy();
  });

  it("offers no verb on an account the reader may not write", async () => {
    stub();
    drawCompany({
      ...company,
      visibility: "owner",
      writable: false,
      owner_id: "someone-else",
    });
    expect(await screen.findByText("Only you")).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: /share with the company/i }),
    ).toBeNull();
  });

  it("draws nothing at all when the server sent no visibility", () => {
    stub();
    const { container } = drawCompany({ ...company, writable: true });
    expect(container.innerHTML).toBe("");
  });
});
