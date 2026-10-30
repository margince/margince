/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { en } from "../../i18n/en";
import { HandoverOffer } from "./handoveroffer";

const ownCompany = {
  id: "co-1",
  owner_id: "u1",
  writable: true,
  archived_at: null,
};

const openOffer = {
  approval_id: "a-1",
  record_type: "company",
  record_id: "co-1",
  to_id: "u-mira",
  status: "pending",
  expires_at: "2026-11-07T09:00:00Z",
};

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

// The server names the colleagues it would accept; the picker offers those
// and no member of the general roster.
const candidates = {
  data: [{ id: "u-mira", display_name: "Mira Voss" }],
};
const roster = {
  data: [{ id: "u-reader", display_name: "Rhea Reader", status: "active" }],
  page: { has_more: false },
};

function serve(offered = false) {
  const sent: { key: string; body?: unknown }[] = [];
  let open = offered;
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
      const text = request ? await request.clone().text() : "";
      sent.push({ key, body: text ? JSON.parse(text) : undefined });
      if (key === "GET /me") {
        return json({
          user: { id: "u1", email: "rep@example.test", full_name: "A Rep" },
          authorization: {
            seat_type: "full",
            objects: { company: { read: true, update: true } },
          },
        });
      }
      if (key === "GET /users/names") {
        return json({ data: [{ id: "u-mira", display_name: "Mira Voss" }] });
      }
      if (key === "GET /users") {
        return json(roster);
      }
      if (key === "GET /records/company/co-1/ownership-offer/candidates") {
        return json(candidates);
      }
      const path = "/records/company/co-1/ownership-offer";
      if (key === `POST ${path}`) {
        open = true;
        return json(openOffer, 201);
      }
      if (key === `DELETE ${path}`) {
        open = false;
        return new Response(null, { status: 204 });
      }
      if (key === `GET ${path}`) {
        return open
          ? json(openOffer)
          : json({ status: 404, title: "Not found" }, 404);
      }
      return json({});
    }),
  );
  return sent;
}

function draw(record: Parameters<typeof HandoverOffer>[0]["record"]) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <HandoverOffer kind="company" record={record} />
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

describe("HandoverOffer", () => {
  it("offers the record to a colleague the server names, then takes the offer back", async () => {
    const sent = serve();
    const user = userEvent.setup();
    draw(ownCompany);

    await user.click(
      await screen.findByRole("button", { name: en["handover.offer"] }),
    );
    await user.click(
      await screen.findByRole("combobox", { name: en["handover.offerTo"] }),
    );
    expect(screen.queryByRole("option", { name: "Rhea Reader" })).toBeNull();
    await user.click(await screen.findByRole("option", { name: "Mira Voss" }));
    await user.type(
      screen.getByRole("textbox", { name: en["handover.offerNote"] }),
      "Yours now",
    );
    await user.click(
      screen.getByRole("button", { name: en["handover.offerSend"] }),
    );

    const post = sent.find(
      (call) => call.key === "POST /records/company/co-1/ownership-offer",
    );
    expect(post?.body).toEqual({ to_id: "u-mira", note: "Yours now" });
    const withdraw = await screen.findByRole("button", {
      name: en["handover.withdrawOffer"],
    });
    expect(
      await screen.findByText(/Offered to Mira Voss\. The offer stays open/),
    ).toBeTruthy();

    await user.click(withdraw);
    expect(sent.map((call) => call.key)).toContain(
      "DELETE /records/company/co-1/ownership-offer",
    );
    expect(
      await screen.findByRole("button", { name: en["handover.offer"] }),
    ).toBeTruthy();
  });

  it.each([
    [
      "a colleague's record",
      { ...ownCompany, owner_id: "u-mira", writable: true },
    ],
    [
      "a record the owner can no longer change",
      { ...ownCompany, writable: false },
    ],
    [
      "an archived record",
      { ...ownCompany, archived_at: "2026-10-01T00:00:00Z" },
    ],
  ] as const)("offers nothing on %s", async (_, record) => {
    const sent = serve();
    draw(record);
    await waitFor(() =>
      expect(sent.map((call) => call.key)).toContain("GET /me"),
    );
    expect(screen.queryByRole("button")).toBeNull();
    expect(sent.map((call) => call.key)).not.toContain(
      "GET /records/company/co-1/ownership-offer",
    );
  });
});
