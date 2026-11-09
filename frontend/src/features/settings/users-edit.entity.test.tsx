// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { UTC_ZONE } from "../../format/timezone";
import { en } from "../../i18n/en";
import { type BackendEntities, backend, render } from "./users-admin.testkit";
import { EditMember } from "./users-edit";
import type { User } from "./users-members";

// The edit dialog names the legal entity a member works for. That is the
// default one when nobody assigned them. A move rides the member edit itself.

const TWO_ENTITIES: BackendEntities = {
  entities: [
    { id: "le-default", is_default: true, display_name: "Acme" },
    { id: "le-vienna", is_default: false, display_name: "Acme Wien" },
  ],
  members: { "le-vienna": ["u-vienna"] },
};

const ASSIGNS = {
  roles: ["admin"],
  allow: {
    user_admin: ["read", "create", "update"],
    legal_entity: ["read", "update"],
  },
};

function member(id: string, name: string): User {
  return {
    id,
    display_name: name,
    email: `${id}@example.test`,
    timezone: UTC_ZONE,
    status: "active",
    email_editable: true,
    is_agent: false,
    roles: ["rep"],
    team_ids: [],
    updated_at: "2026-10-01T09:00:00Z",
    allowed_actions: ["edit", "change_role"],
  };
}

const entityField = () =>
  screen.findByRole("combobox", { name: en["users.entityLabel"] });

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("EditMember legal entity", () => {
  it("shows the entity a member is assigned to", async () => {
    vi.stubGlobal("fetch", backend([], ASSIGNS, TWO_ENTITIES));
    render(
      <EditMember
        member={member("u-vienna", "Vera Vienna")}
        onClose={vi.fn()}
      />,
    );
    expect(await entityField()).toHaveTextContent("Acme Wien");
  });

  it("shows the default entity for a member nobody assigned", async () => {
    vi.stubGlobal("fetch", backend([], ASSIGNS, TWO_ENTITIES));
    render(
      <EditMember member={member("u-plain", "Paul Plain")} onClose={vi.fn()} />,
    );
    const field = await entityField();
    await waitFor(() => expect(field).toHaveTextContent("Acme"));
    expect(field).not.toHaveTextContent("Acme Wien");
  });

  it("saves a change in the one member edit", async () => {
    const user = userEvent.setup();
    const calls: { method: string; url: string; body?: unknown }[] = [];
    vi.stubGlobal("fetch", backend(calls, ASSIGNS, TWO_ENTITIES));
    const closed = vi.fn();
    render(
      <EditMember member={member("u-plain", "Paul Plain")} onClose={closed} />,
    );
    await pickOption(user, await entityField(), "Acme Wien");
    await user.click(screen.getByRole("button", { name: "Save member" }));
    await waitFor(() => expect(closed).toHaveBeenCalledOnce());
    const writes = calls.filter((call) => call.method !== "GET");
    expect(writes).toEqual([
      {
        method: "PATCH",
        url: expect.stringMatching(/\/users\/u-plain$/),
        body: expect.objectContaining({ legal_entity_id: "le-vienna" }),
      },
    ]);
  });

  it("sends no entity when it is kept", async () => {
    const user = userEvent.setup();
    const calls: { method: string; url: string; body?: unknown }[] = [];
    vi.stubGlobal("fetch", backend(calls, ASSIGNS, TWO_ENTITIES));
    const closed = vi.fn();
    render(
      <EditMember
        member={member("u-vienna", "Vera Vienna")}
        onClose={closed}
      />,
    );
    expect(await entityField()).toHaveTextContent("Acme Wien");
    const name = screen.getByRole("textbox", { name: "Full name" });
    await user.type(name, " Jr");
    await user.click(screen.getByRole("button", { name: "Save member" }));
    await waitFor(() => expect(closed).toHaveBeenCalledOnce());
    const patch = calls.find((call) => call.method === "PATCH");
    expect(patch?.body).not.toHaveProperty("legal_entity_id");
  });

  it("is not asked of a reader who may not assign members", async () => {
    vi.stubGlobal(
      "fetch",
      backend(
        [],
        {
          roles: ["admin"],
          allow: { user_admin: ["read", "update"], legal_entity: ["read"] },
        },
        TWO_ENTITIES,
      ),
    );
    render(
      <EditMember member={member("u-plain", "Paul Plain")} onClose={vi.fn()} />,
    );
    await screen.findByRole("textbox", { name: "Full name" });
    expect(
      screen.queryByRole("combobox", { name: en["users.entityLabel"] }),
    ).toBeNull();
  });
});
