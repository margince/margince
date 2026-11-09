// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import type { GrantSpec } from "../../testing/mefixture";
import { meFixture } from "../../testing/mefixture";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { InviteUserForm } from "./users-invite-form";

// The invite form asks which legal entity the member works for once there is
// a second one to choose and the inviter may assign members. It starts on the
// default entity, and an invite left there names none.

const ACME = { id: "le-default", is_default: true, display_name: "Acme" };
const WIEN = { id: "le-vienna", is_default: false, display_name: "Acme Wien" };

const ASSIGNS: GrantSpec = {
  user_admin: ["read", "create", "update"],
  legal_entity: ["read", "update"],
};

function renderForm(
  entities: readonly (typeof ACME)[],
  allow: GrantSpec,
): { invites: unknown[]; entityReads: () => number } {
  const invites: unknown[] = [];
  let entityReads = 0;
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture({ roles: ["admin"], allow })),
    "GET /teams": () =>
      jsonResponse({ data: [], page: { has_more: false, next_cursor: null } }),
    "GET /users/assignable-roles": () =>
      jsonResponse({ roles: [{ key: "rep", name: "User", is_system: true }] }),
    "GET /users/access-preview": () =>
      jsonResponse({ role: "rep", row_scope: "own", objects: {} }),
    "GET /legal-entities": () => {
      entityReads += 1;
      return jsonResponse({ data: entities });
    },
    "POST /users": (body) => {
      invites.push(body);
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
  return { invites, entityReads: () => entityReads };
}

async function fillIn(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText(/email/i), "new@acme.test");
  await user.type(screen.getByLabelText(/full name/i), "New Colleague");
  await waitFor(() =>
    expect(screen.getByRole("button", { name: /invite/i })).toBeEnabled(),
  );
}

const entityPicker = () =>
  screen.queryByRole("combobox", { name: en["users.entityLabel"] });

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("InviteUserForm legal entity", () => {
  it("starts on the default entity and sends none when it is kept", async () => {
    const user = userEvent.setup();
    const { invites } = renderForm([ACME, WIEN], ASSIGNS);
    await fillIn(user);
    await waitFor(() => expect(entityPicker()).toHaveTextContent("Acme"));
    await user.click(screen.getByRole("button", { name: /invite/i }));
    await waitFor(() => expect(invites).toHaveLength(1));
    expect(invites[0]).not.toHaveProperty("legal_entity_id");
  });

  it("sends the entity chosen", async () => {
    const user = userEvent.setup();
    const { invites } = renderForm([ACME, WIEN], ASSIGNS);
    await fillIn(user);
    const picker = await screen.findByRole("combobox", {
      name: en["users.entityLabel"],
    });
    await pickOption(user, picker, "Acme Wien");
    await user.click(screen.getByRole("button", { name: /invite/i }));
    await waitFor(() => expect(invites).toHaveLength(1));
    expect(invites[0]).toMatchObject({ legal_entity_id: "le-vienna" });
  });

  it("is not asked while the installation trades as one entity", async () => {
    const user = userEvent.setup();
    const { entityReads } = renderForm([ACME], ASSIGNS);
    await fillIn(user);
    await waitFor(() => expect(entityReads()).toBeGreaterThan(0));
    expect(entityPicker()).toBeNull();
  });

  it("is not asked of an inviter who may not assign members", async () => {
    const user = userEvent.setup();
    renderForm([ACME, WIEN], {
      user_admin: ["read", "create", "update"],
      legal_entity: ["read"],
    });
    await fillIn(user);
    expect(entityPicker()).toBeNull();
  });
});
