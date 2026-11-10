// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { components } from "../../api/schema";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { jsonResponse } from "../../testing/story-utils";
import { useContactEdit } from "./contactedit";

type Contact = components["schemas"]["Contact"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

const READER = "u-reader";
const COLLEAGUE = "u-colleague";

const contact: Contact = {
  id: "p-1",
  full_name: "Dana Buyer",
  source: "manual",
  captured_by: "human:u-reader",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
  version: 4,
  visibility: "owner",
  owner_id: READER,
  writable: true,
};

// The server as the giver sees it: once the owner moves, the private
// contact reads as not found unless `stillShared` keeps it in reach.
function serve(stillShared: boolean) {
  let handedOn = false;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : new Request(input);
      const path = new URL(request.url).pathname.replace(/^\/v1/, "");
      if (path === "/me") {
        return jsonResponse({ user: { id: READER, full_name: "A Rep" } });
      }
      if (path === "/contacts/p-1" && request.method === "PATCH") {
        handedOn = true;
        return jsonResponse({ ...contact, owner_id: COLLEAGUE, version: 5 });
      }
      if (path === "/contacts/p-1" && handedOn && !stillShared) {
        return jsonResponse({ status: 404, title: "Not found" }, 404);
      }
      if (path === "/contacts/p-1") {
        return jsonResponse({ ...contact, owner_id: COLLEAGUE, version: 5 });
      }
      return jsonResponse({ data: [], page: { has_more: false } });
    }),
  );
}

function wrapper({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <ToastProvider>
          {children}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>
  );
}

async function handOn() {
  window.location.hash = "#/contacts/p-1";
  const { result } = renderHook(() => useContactEdit(contact), { wrapper });
  await act(async () => {
    await result.current.save({ owner_id: COLLEAGUE }, {}, contact);
  });
}

describe("handing a private contact to a colleague", () => {
  it("takes the giver to the list and says why, once the contact is out of reach", async () => {
    serve(false);
    await handOn();

    expect(window.location.hash).toBe("#/contacts");
    expect(
      await screen.findByText(/now owns this contact\. It is private to them/),
    ).toBeTruthy();
  });

  it("stays on the contact when a share keeps it in reach", async () => {
    serve(true);
    await handOn();

    expect(window.location.hash).toBe("#/contacts/p-1");
  });
});
