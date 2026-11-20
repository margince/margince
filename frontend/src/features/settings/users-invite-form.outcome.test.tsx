// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type Locale, LocaleProvider, translate } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { InviteUserForm } from "./users-invite-form";

// What the invite form says when the answer names no member. The admin learns
// whether to look at the list or to try again.

function renderForm(
  answer: () => Response | Promise<Response>,
  locale: Locale,
) {
  const sent: unknown[] = [];
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
  const invalidated = vi.spyOn(client, "invalidateQueries");
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>
        <InviteUserForm askName={false} onInvited={() => undefined} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { sent, invalidated };
}

async function fillAndFind(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByRole("textbox"), "lan@acme.test");
  const invite = screen.getByRole("button", { name: /invite|einladen|mời/i });
  await waitFor(() => expect(invite).toBeEnabled());
  return invite;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("InviteUserForm when the answer is not a member", () => {
  it("sends the admin to the list after a server failure, and refreshes it", async () => {
    const user = userEvent.setup();
    const { invalidated } = renderForm(
      () => jsonResponse({ title: "Internal", status: 500 }, 500),
      "en",
    );
    await user.click(await fillAndFind(user));

    expect(
      await screen.findByText(translate("en", "users.inviteUnconfirmed")),
    ).toBeInTheDocument();
    expect(invalidated).toHaveBeenCalled();
  });

  it("names a waiting invitation in the reader's language", async () => {
    const user = userEvent.setup();
    const { invalidated } = renderForm(
      () =>
        jsonResponse(
          { title: "Conflict", status: 409, code: "already_invited" },
          409,
        ),
      "de",
    );
    await user.click(await fillAndFind(user));

    expect(
      await screen.findByText(translate("de", "users.alreadyInvited")),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(translate("de", "users.inviteUnconfirmed")),
    ).not.toBeInTheDocument();
    expect(invalidated).not.toHaveBeenCalled();
  });
});
