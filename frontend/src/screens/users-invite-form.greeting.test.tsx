// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { installFetchStub, jsonResponse } from "./story-utils";
import { InviteUserForm } from "./users-invite-form";

// The invite form's optional greeting name: sent when the admin typed one, and
// left off the wire when they did not, so the member's first sign-in can fill it.

function renderForm(sent: unknown[]) {
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture({ roles: ["admin"], allow: {} })),
    "GET /teams": () =>
      jsonResponse({ data: [], page: { has_more: false, next_cursor: null } }),
    "GET /users/assignable-roles": () =>
      jsonResponse({ roles: [{ key: "rep", name: "User", is_system: true }] }),
    "GET /users/access-preview": () =>
      jsonResponse({ role: "rep", row_scope: "own", objects: {} }),
    "POST /users": (body) => {
      sent.push(body);
      return jsonResponse({ id: "u-new" }, 201);
    },
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <InviteUserForm onInvited={() => undefined} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("InviteUserForm greeting name", () => {
  it("sends the greeting name the admin typed, trimmed", async () => {
    const user = userEvent.setup();
    const sent: unknown[] = [];
    renderForm(sent);
    await user.type(screen.getByLabelText(/email/i), "lan@acme.test");
    await user.type(screen.getByLabelText(/full name/i), "Nguyễn Thị Lan");
    await user.type(screen.getByLabelText(/greeting name/i), " Lan ");
    const invite = screen.getByRole("button", { name: /invite/i });
    await waitFor(() => expect(invite).toBeEnabled());
    await user.click(invite);

    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]).toMatchObject({
      display_name: "Nguyễn Thị Lan",
      greeting_name: "Lan",
    });
  });

  it("leaves the greeting name off the wire when none was typed", async () => {
    const user = userEvent.setup();
    const sent: unknown[] = [];
    renderForm(sent);
    await user.type(screen.getByLabelText(/email/i), "ada@acme.test");
    await user.type(screen.getByLabelText(/full name/i), "Ada Lovelace");
    const invite = screen.getByRole("button", { name: /invite/i });
    await waitFor(() => expect(invite).toBeEnabled());
    await user.click(invite);

    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]).not.toHaveProperty("greeting_name");
  });
});
