// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { pickOption } from "../design-system/select-testing";
import { LocaleProvider } from "../i18n";
import { installFetchStub, jsonResponse } from "./story-utils";
import { InviteUserForm } from "./users-invite-form";

// The invite form's role: it starts on the ordinary seat when the reader may
// hand it out, and otherwise sends nothing until the reader picks a role they
// are offered. An invite the server would refuse is never one click away.

const FIELD_SALES = {
  key: "custom_field_sales",
  name: "Field sales",
  is_system: false,
};

function renderForm(offered: readonly (typeof FIELD_SALES)[]) {
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture({ allow: {} })),
    "GET /teams": () => jsonResponse({ data: [], next_cursor: null }),
    "GET /users/assignable-roles": () => jsonResponse({ roles: offered }),
    "GET /users/access-preview": () =>
      jsonResponse({ role: "rep", row_scope: "own", objects: {} }),
  });
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
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

describe("InviteUserForm role", () => {
  it("waits for a role the reader is offered when the ordinary seat is not", async () => {
    const user = userEvent.setup();
    renderForm([FIELD_SALES]);
    await user.type(screen.getByLabelText(/email/i), "new@acme.test");
    await user.type(screen.getByLabelText(/full name/i), "New Colleague");
    const role = await screen.findByRole("combobox", { name: /role/i });
    await waitFor(() => expect(role).not.toBeDisabled());

    expect(role).toHaveTextContent("Set role…");
    expect(screen.getByRole("button", { name: /invite/i })).toBeDisabled();

    await pickOption(user, role, "Field sales");
    expect(screen.getByRole("button", { name: /invite/i })).toBeEnabled();
  });

  it("starts on the ordinary seat when the reader may hand it out", async () => {
    const user = userEvent.setup();
    renderForm([FIELD_SALES, { key: "rep", name: "User", is_system: true }]);
    await user.type(screen.getByLabelText(/email/i), "new@acme.test");
    await user.type(screen.getByLabelText(/full name/i), "New Colleague");
    const role = await screen.findByRole("combobox", { name: /role/i });
    await waitFor(() => expect(role).toHaveTextContent("User"));
    expect(screen.getByRole("button", { name: /invite/i })).toBeEnabled();
  });
});
