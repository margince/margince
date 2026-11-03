/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { en } from "../../i18n/en";
import { openChoiceDialog } from "../../testing/choicedialog.testkit";
import { UsersAdminCard } from "./users-admin";
import {
  type BackendEntities,
  backend,
  render,
  rowFor,
} from "./users-admin.testkit";

// The roster's legal entity column shows once there is a second entity to
// choose. Every move goes through the members endpoint of the entity chosen.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const TWO_ENTITIES: BackendEntities = {
  entities: [
    { id: "le-default", is_default: true, display_name: "Acme" },
    { id: "le-vienna", is_default: false, display_name: "Acme Wien" },
  ],
  members: { "le-vienna": ["u-none"] },
};

const ADMIN = {
  roles: ["admin"],
  allow: {
    user_admin: ["read", "create", "update", "delete"],
    legal_entity: ["read", "create", "update", "delete"],
  },
};

const entityOf = (name: string) => new RegExp(`${name}’s legal entity`, "i");

describe("UsersAdminCard legal entity column", () => {
  it("is not drawn while the installation trades as one entity", async () => {
    vi.stubGlobal("fetch", backend([], ADMIN));
    render(<UsersAdminCard />);
    await screen.findByText("Ada Active");
    expect(
      screen.queryByRole("columnheader", { name: en["users.entityLabel"] }),
    ).toBeNull();
  });

  it("names each member's entity, and the default for one nobody assigned", async () => {
    vi.stubGlobal("fetch", backend([], ADMIN, TWO_ENTITIES));
    render(<UsersAdminCard />);
    await screen.findByRole("columnheader", { name: en["users.entityLabel"] });
    await waitFor(() =>
      expect(within(rowFor("Nora None")).getByText("Acme Wien")).toBeTruthy(),
    );
    expect(within(rowFor("Ada Active")).getByText("Acme")).toBeTruthy();
  });

  it("moves a member by assigning them to the entity chosen", async () => {
    const user = userEvent.setup();
    const calls: { method: string; url: string; body?: unknown }[] = [];
    vi.stubGlobal("fetch", backend(calls, ADMIN, TWO_ENTITIES));
    render(<UsersAdminCard />);
    await waitFor(() =>
      expect(within(rowFor("Ada Active")).getByText("Acme")).toBeTruthy(),
    );
    const { dialog, chooser, save } = await openChoiceDialog(
      user,
      entityOf("Ada Active"),
      within(rowFor("Ada Active")).getByRole("button", {
        name: /legal entity/i,
      }),
    );
    await pickOption(user, chooser, "Acme Wien");
    await user.click(save);
    await waitFor(() => expect(dialog.isConnected).toBe(false));
    expect(calls).toContainEqual({
      method: "PUT",
      url: expect.stringMatching(/\/legal-entities\/le-vienna\/members$/),
      body: { user_id: "u-active" },
    });
  });

  it("shows the entity as words to an admin who may not assign one", async () => {
    vi.stubGlobal(
      "fetch",
      backend(
        [],
        {
          roles: ["admin"],
          allow: {
            user_admin: ["read", "create", "update", "delete"],
            legal_entity: ["read"],
          },
        },
        TWO_ENTITIES,
      ),
    );
    render(<UsersAdminCard />);
    await waitFor(() =>
      expect(within(rowFor("Nora None")).getByText("Acme Wien")).toBeTruthy(),
    );
    expect(
      within(rowFor("Nora None")).queryByRole("button", {
        name: /legal entity/i,
      }),
    ).toBeNull();
  });
});
