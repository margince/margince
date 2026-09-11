/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { components } from "../api/schema";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { ContactAccess } from "./contactaccess";

type Contact = components["schemas"]["Contact"];

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

function stub(sent: string[] = []) {
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
      return json({});
    }),
  );
}

function draw(contact: Contact) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <ContactAccess contact={contact} />
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

describe("ContactAccess", () => {
  it("says a captured contact is private to its owner, which no other surface does", async () => {
    stub();
    draw({ ...base, visibility: "owner", writable: true, owner_id: "u1" });
    expect(await screen.findByText(/private to its owner/i)).toBeTruthy();
  });

  it("says a promoted contact is the company's", async () => {
    stub();
    draw({ ...base, visibility: "workspace", writable: true });
    expect(await screen.findByText(/everyone in the company/i)).toBeTruthy();
  });

  it("publishes a private contact through the ordinary contact patch", async () => {
    const sent: string[] = [];
    stub(sent);
    draw({ ...base, visibility: "owner", writable: true, owner_id: "u1" });
    await userEvent.click(
      await screen.findByRole("button", {
        name: /share with the company/i,
      }),
    );
    expect(sent).toContain("PATCH /contacts/p-1");
  });

  it("makes a workspace contact private again — the direction that did not exist", async () => {
    // The whole point of the change. A contact the sender classifier published
    // with nobody approving it could not be narrowed by anybody, its own owner
    // included.
    const sent: string[] = [];
    stub(sent);
    draw({ ...base, visibility: "workspace", writable: true, owner_id: "u1" });
    await userEvent.click(
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
    expect(await screen.findByText(/private to its owner/i)).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: /share with the company/i }),
    ).toBeNull();
  });

  it("offers the verb to a colleague holding a write grant", async () => {
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
    await userEvent.click(
      await screen.findByRole("button", {
        name: /share with the company/i,
      }),
    );
    expect(sent).toContain("PATCH /contacts/p-1");
  });

  it("offers no verb on any contact to a reader who cannot write it", async () => {
    stub();
    draw({ ...base, visibility: "workspace", writable: false, owner_id: "u1" });
    expect(await screen.findByText(/everyone in the company/i)).toBeTruthy();
    expect(screen.queryByRole("button", { name: /make private/i })).toBeNull();
  });

  it("refuses to write a row it read back without a version", async () => {
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
    await userEvent.click(
      await screen.findByRole("button", { name: /make private/i }),
    );
    expect(sent).not.toContain("PATCH /contacts/p-1");
  });

  it("draws nothing at all when the server sent no visibility", () => {
    // A server too old to send it, or a path that does not. Guessing
    // `workspace` would tell a reader their private contact is public.
    stub();
    const { container } = draw({ ...base, writable: true });
    expect(container.innerHTML).toBe("");
  });
});
