// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { meFixture } from "../../testing/mefixture";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { InviteUserForm } from "./users-invite-form";

function renderForm(sent: unknown[], answer: () => Response) {
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
      return answer();
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

const created = () => jsonResponse({ id: "u-new" }, 201);

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("InviteUserForm refusals", () => {
  it("asks for the address and the name on their fields and sends nothing", async () => {
    const user = userEvent.setup();
    const sent: unknown[] = [];
    renderForm(sent, created);
    await user.click(screen.getByRole("button", { name: /invite/i }));
    expect(await screen.findByText("Email is required.")).toBeVisible();
    expect(screen.getByText("Full name is required.")).toBeVisible();
    const email = screen.getByRole("textbox", { name: "Email" });
    await waitFor(() => expect(email).toHaveFocus());
    expect(sent).toEqual([]);
  });

  it("refuses an address with no @ before sending, and clears once it is edited", async () => {
    const user = userEvent.setup();
    const sent: unknown[] = [];
    renderForm(sent, created);
    const email = screen.getByRole("textbox", { name: "Email" });
    await user.type(email, "ada.acme.test");
    await user.type(
      screen.getByRole("textbox", { name: "Full name" }),
      "Ada Lovelace",
    );
    await user.click(screen.getByRole("button", { name: /invite/i }));
    expect(
      await screen.findByText("Enter an email address for Email."),
    ).toBeVisible();
    expect(email).toHaveAttribute("aria-invalid", "true");
    await user.type(email, "x");
    expect(screen.queryByText("Enter an email address for Email.")).toBeNull();
    expect(sent).toEqual([]);
  });

  it("puts an address another member holds on the email field", async () => {
    const user = userEvent.setup();
    const sent: unknown[] = [];
    renderForm(sent, () =>
      jsonResponse(
        {
          status: 409,
          code: "email_taken",
          detail: "a user with this email already exists in this company",
        },
        409,
      ),
    );
    const email = screen.getByRole("textbox", { name: "Email" });
    await user.type(email, "ada@acme.test");
    await user.type(
      screen.getByRole("textbox", { name: "Full name" }),
      "Ada Lovelace",
    );
    await user.click(screen.getByRole("button", { name: /invite/i }));
    expect(await screen.findByText(en["users.emailTaken"])).toBeVisible();
    expect(email).toHaveAttribute("aria-invalid", "true");
    await waitFor(() => expect(email).toHaveFocus());
    expect(screen.queryByText(/already exists in this company/)).toBeNull();
  });

  it("drops the foot's refusal once the reader edits a field", async () => {
    const user = userEvent.setup();
    const sent: unknown[] = [];
    renderForm(sent, () =>
      jsonResponse(
        {
          status: 409,
          code: "conflict",
          detail: "This sign-in address is already used.",
        },
        409,
      ),
    );
    const email = screen.getByRole("textbox", { name: "Email" });
    await user.type(email, "ada@acme.test");
    await user.type(
      screen.getByRole("textbox", { name: "Full name" }),
      "Ada Lovelace",
    );
    await user.click(screen.getByRole("button", { name: /invite/i }));
    expect(
      await screen.findByText("This sign-in address is already used."),
    ).toBeVisible();
    expect(screen.queryByText(en["users.emailTaken"])).toBeNull();
    await user.type(email, "x");
    expect(
      screen.queryByText("This sign-in address is already used."),
    ).toBeNull();
  });
});
