/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { components } from "../api/schema";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { PersonAccess } from "./personaccess";

type Person = components["schemas"]["Person"];

const base: Person = {
  id: "p-1",
  full_name: "Dana Buyer",
  source: "gmail:seed",
  captured_by: "connector:gmail",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
};

const seatMayWrite = {
  user: { id: "u1", email: "rep@example.test", full_name: "A Rep" },
  authorization: {
    seat_type: "full",
    objects: { person: { read: true, update: true } },
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

function draw(person: Person) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <PersonAccess person={person} />
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

describe("PersonAccess", () => {
  it("says a captured contact is private to the reader, which no other surface does", async () => {
    stub();
    draw({ ...base, visibility: "owner", writable: true });
    expect(await screen.findByText(/private to you/i)).toBeTruthy();
  });

  it("says a promoted contact is the organization's", async () => {
    stub();
    draw({ ...base, visibility: "workspace", writable: true });
    expect(
      await screen.findByText(/everyone in the organization/i),
    ).toBeTruthy();
  });

  it("offers the owner the one verb that changes the answer", async () => {
    const sent: string[] = [];
    stub(sent);
    draw({ ...base, visibility: "owner", writable: true });
    await userEvent.click(
      await screen.findByRole("button", {
        name: /share with the organization/i,
      }),
    );
    expect(sent).toContain("POST /people/p-1/publish");
  });

  it("offers no verb to a reader who is not the owner", async () => {
    stub();
    draw({ ...base, visibility: "owner", writable: false });
    expect(await screen.findByText(/private to you/i)).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: /share with the organization/i }),
    ).toBeNull();
  });

  it("offers no verb on a contact the organization can already see", async () => {
    stub();
    draw({ ...base, visibility: "workspace", writable: true });
    expect(
      await screen.findByText(/everyone in the organization/i),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: /share with the organization/i }),
    ).toBeNull();
  });

  it("draws nothing at all when the server sent no visibility", () => {
    // A server too old to send it, or a path that does not. Guessing
    // `workspace` would tell a reader their private contact is public.
    stub();
    const { container } = draw({ ...base, writable: true });
    expect(container.innerHTML).toBe("");
  });
});
