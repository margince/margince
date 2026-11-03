// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { ContactsScreen } from "./contacts";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const emptyPage = { data: [], page: { next_cursor: null, has_more: false } };

function leadHolds(details: Record<string, unknown>) {
  return jsonResponse(
    {
      type: "https://errors.gradion.com/lead_holds_email",
      title: "Conflict",
      status: 409,
      code: "lead_holds_email",
      detail: "a live record with this key already exists",
      details,
    },
    409,
  );
}

// Answers the first write with the lead-held refusal and any write that names
// the leads to convert with the created contact, recording every body.
function stubWrites(path: string, details: Record<string, unknown>) {
  const posted: Record<string, unknown>[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url);
      if (request.method === "POST" && url.pathname.endsWith(path)) {
        const body = JSON.parse(await request.text());
        posted.push(body);
        if (!body.convert_lead_ids) return leadHolds(details);
        const contact = { id: "p-new", full_name: "Dana Buyer", version: 1 };
        return jsonResponse(
          path === "/quick-capture" ? { contact } : contact,
          201,
        );
      }
      if (url.pathname.endsWith("/me")) {
        return jsonResponse(
          meFixture({ allow: { contact: ["read", "create"] } }),
        );
      }
      return jsonResponse(emptyPage);
    }),
  );
  return posted;
}

async function submit(button: string) {
  const user = userEvent.setup();
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ContactsScreen />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await user.click(screen.getByTestId(button));
  await user.type(screen.getByLabelText("Full name *"), "Dana Buyer");
  await user.click(screen.getByRole("button", { name: "Create" }));
  await screen.findByText("This is already a lead.");
  return user;
}

describe("a contact form over a live lead", () => {
  it("New contact converts the lead by sending the form again", async () => {
    const posted = stubWrites("/contacts", {
      existing_id: "lead-1",
      lead_ids: ["lead-1"],
      convertible: true,
      field: "emails",
    });
    const user = await submit("new-record");
    await user.click(screen.getByRole("button", { name: "Convert the lead" }));
    await waitFor(() => expect(posted).toHaveLength(2));
    expect(posted[1]).toMatchObject({
      full_name: "Dana Buyer",
      convert_lead_ids: ["lead-1"],
    });
    expect(posted[0]).not.toHaveProperty("convert_lead_ids");
  });

  it("Quick capture converts the lead by sending the form again", async () => {
    const posted = stubWrites("/quick-capture", {
      existing_id: "lead-2",
      lead_ids: ["lead-2"],
      convertible: true,
      field: "email",
    });
    const user = await submit("quick-capture");
    await user.click(screen.getByRole("button", { name: "Convert the lead" }));
    await waitFor(() => expect(posted).toHaveLength(2));
    expect(posted[1]).toMatchObject({ convert_lead_ids: ["lead-2"] });
  });

  it("opens the lead instead of offering a convert the reader may not make", async () => {
    stubWrites("/contacts", {
      existing_id: "lead-3",
      lead_ids: ["lead-3"],
      convertible: false,
      field: "emails",
    });
    const user = await submit("new-record");
    expect(
      screen.queryByRole("button", { name: "Convert the lead" }),
    ).toBeNull();
    expect(
      screen.getByText("Ask the lead’s owner to convert the lead."),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: "View existing record" }),
    ).toBeNull();
    await user.click(screen.getByRole("button", { name: "Open the lead" }));
    expect(window.location.hash).toBe("#/leads/lead-3");
  });

  it("names no lead the reader cannot open", async () => {
    stubWrites("/contacts", {});
    await submit("new-record");
    expect(screen.queryByRole("button", { name: "Open the lead" })).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Convert the lead" }),
    ).toBeNull();
  });
});
