/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ToastProvider, ToastRegion } from "../design-system/toast";
import { en } from "../i18n/en";
import type { EditAccessRecordType } from "../lib/editaccessapi";
import { useMe } from "../lib/session";
import { EditAccessRequest } from "./editaccessrequest";

type Seat = "full" | "read";

const colleaguesLead = {
  id: "le-1",
  owner_id: "u-owner",
  writable: false,
  archived_at: null,
};

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

// The server as this control meets it: the viewer, the owner's name, and the
// viewer's own request on one record. The request exists once POST is sent.
function serve(kind: EditAccessRecordType, seat: Seat = "full", asked = false) {
  const sent: string[] = [];
  let requested = asked;
  const path = `/records/${kind}/le-1/edit-access-request`;
  const open = {
    approval_id: "a-1",
    record_type: kind,
    record_id: "le-1",
    owner_id: "u-owner",
    status: "pending",
    expires_at: "2026-11-07T09:00:00Z",
  };
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
      if (key === "GET /me") {
        return json({
          user: { id: "u1", email: "rep@example.test", full_name: "A Rep" },
          authorization: {
            seat_type: seat,
            objects: { [kind]: { read: true, update: true } },
          },
        });
      }
      if (key === "GET /users/names") {
        return json({ data: [{ id: "u-owner", display_name: "Mira Voss" }] });
      }
      if (key === `POST ${path}`) {
        requested = true;
        return json(open, 201);
      }
      if (key === `DELETE ${path}`) {
        requested = false;
        return new Response(null, { status: 204 });
      }
      if (key === `GET ${path}`) {
        return requested
          ? json(open)
          : json({ status: 404, title: "Not found" }, 404);
      }
      return json({});
    }),
  );
  return sent;
}

// Shows once the viewer's seat and grants are read, which is when the ask
// decides whether to read the open request.
function SeatRead() {
  return useMe().data ? <span>seat read</span> : null;
}

function draw(
  kind: EditAccessRecordType,
  record: Parameters<typeof EditAccessRequest>[0]["record"],
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <EditAccessRequest kind={kind} record={record} />
        <SeatRead />
        <ToastRegion />
      </ToastProvider>
    </QueryClientProvider>,
  );
}

const askLabel = en["editAccess.ask"].replace("{owner}", "Mira Voss");

beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("EditAccessRequest", () => {
  it.each(["lead", "deal", "project"] as const)(
    "asks the owner of a %s, then takes the request back",
    async (kind) => {
      const sent = serve(kind);
      const user = userEvent.setup();
      draw(kind, colleaguesLead);

      await user.click(await screen.findByRole("button", { name: askLabel }));
      expect(sent).toContain(`POST /records/${kind}/le-1/edit-access-request`);
      const withdraw = await screen.findByRole("button", {
        name: en["editAccess.withdraw"],
      });
      expect(
        screen.getByText(/You asked Mira Voss for edit access/),
      ).toBeTruthy();

      await user.click(withdraw);
      expect(sent).toContain(
        `DELETE /records/${kind}/le-1/edit-access-request`,
      );
      expect(
        await screen.findByRole("button", { name: askLabel }),
      ).toBeTruthy();
    },
  );

  it("shows the open request a reader made earlier", async () => {
    serve("lead", "full", true);
    draw("lead", colleaguesLead);
    expect(
      await screen.findByRole("button", { name: en["editAccess.withdraw"] }),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: askLabel })).toBeNull();
  });

  it.each([
    [
      "a lead the reader can already change",
      { ...colleaguesLead, writable: true },
      "full",
    ],
    ["the reader's own lead", { ...colleaguesLead, owner_id: "u1" }, "full"],
    ["a lead nobody owns", { ...colleaguesLead, owner_id: null }, "full"],
    [
      "a closed lead",
      { ...colleaguesLead, archived_at: "2026-10-01T00:00:00Z" },
      "full",
    ],
    ["a read seat", colleaguesLead, "read"],
  ] as const)("offers nothing on %s", async (_, record, seat) => {
    const sent = serve("lead", seat);
    draw("lead", record);
    await screen.findByText("seat read");
    expect(sent).not.toContain("GET /records/lead/le-1/edit-access-request");
    expect(screen.queryByRole("button")).toBeNull();
  });
});
