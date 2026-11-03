/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { en } from "../../i18n/en";
import { HandoverRequest } from "./handoverrequest";

type Seat = "full" | "read";

const colleaguesCompany = {
  id: "co-1",
  owner_id: "u-owner",
  writable: false,
  archived_at: null,
};

const openRequest = {
  approval_id: "a-1",
  record_type: "company",
  record_id: "co-1",
  owner_id: "u-owner",
  status: "pending",
  expires_at: "2026-11-07T09:00:00Z",
};

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

// The server as this control meets it: the viewer, the owner's name, and the
// viewer's own request. The request exists once POST has been sent.
function serve(seat: Seat = "full", asked = false) {
  const sent: string[] = [];
  let requested = asked;
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
            objects: { company: { read: true, update: true } },
          },
        });
      }
      if (key === "GET /users/names") {
        return json({ data: [{ id: "u-owner", display_name: "Mira Voss" }] });
      }
      const path = "/records/company/co-1/ownership-request";
      if (key === `POST ${path}`) {
        requested = true;
        return json(openRequest, 201);
      }
      if (key === `DELETE ${path}`) {
        requested = false;
        return new Response(null, { status: 204 });
      }
      if (key === `GET ${path}`) {
        return requested
          ? json(openRequest)
          : json({ status: 404, title: "Not found" }, 404);
      }
      return json({});
    }),
  );
  return sent;
}

function draw(record: Parameters<typeof HandoverRequest>[0]["record"]) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <HandoverRequest kind="company" record={record} />
        <ToastRegion />
      </ToastProvider>
    </QueryClientProvider>,
  );
}

const askLabel = en["handover.ask"].replace("{owner}", "Mira Voss");

beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("HandoverRequest", () => {
  it("asks the owner, then says the request is open and takes it back", async () => {
    const sent = serve();
    const user = userEvent.setup();
    draw(colleaguesCompany);

    await user.click(await screen.findByRole("button", { name: askLabel }));
    expect(sent).toContain("POST /records/company/co-1/ownership-request");
    const withdraw = await screen.findByRole("button", {
      name: en["handover.withdraw"],
    });
    expect(screen.getByText(/You asked Mira Voss/)).toBeTruthy();

    await user.click(withdraw);
    expect(sent).toContain("DELETE /records/company/co-1/ownership-request");
    expect(await screen.findByRole("button", { name: askLabel })).toBeTruthy();
  });

  it("shows the open request a reader made earlier", async () => {
    serve("full", true);
    draw(colleaguesCompany);
    expect(
      await screen.findByRole("button", { name: en["handover.withdraw"] }),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: askLabel })).toBeNull();
  });

  it.each([
    [
      "a record the reader can change",
      { ...colleaguesCompany, writable: true },
      "full",
    ],
    [
      "the reader's own record",
      { ...colleaguesCompany, owner_id: "u1" },
      "full",
    ],
    ["a record nobody owns", { ...colleaguesCompany, owner_id: null }, "full"],
    [
      "an archived record",
      { ...colleaguesCompany, archived_at: "2026-10-01T00:00:00Z" },
      "full",
    ],
    ["a read seat", colleaguesCompany, "read"],
  ] as const)("offers nothing on %s", async (_, record, seat) => {
    const sent = serve(seat);
    draw(record);
    await waitFor(() => expect(sent).toContain("GET /me"));
    expect(screen.queryByRole("button")).toBeNull();
    expect(sent).not.toContain("GET /records/company/co-1/ownership-request");
  });
});
