/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { jsonResponse, render } from "./settings.testkit";
import { NewTeamAction, RenameTeamAction } from "./team-dialogs";

// A team name is unique, and the server answers a second one with a 409 whose
// English detail names the store, not the reader's next step.
const TAKEN = {
  type: "about:blank",
  title: "Conflict",
  status: 409,
  code: "conflict",
  detail: 'conflict: a team named "Nord" already exists',
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it.each<[string, ReactNode, string]>([
  ["create", <NewTeamAction key="create" />, en["users.newTeamOpen"]],
  [
    "rename",
    <RenameTeamAction key="rename" team={{ id: "t-1", name: "Süd" }} />,
    en["users.teamRename"],
  ],
])(
  "a %s refused as a duplicate says so on the name field until it changes",
  async (_verb, action, opener) => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse(TAKEN, 409)),
    );
    render(action);
    await user.click(screen.getByRole("button", { name: opener }));
    const dialog = within(await screen.findByRole("dialog"));
    const field = dialog.getByLabelText(en["users.teamNameLabel"]);
    await user.clear(field);
    await user.type(field, "Nord{Enter}");

    await vi.waitFor(() =>
      expect(field).toHaveAccessibleDescription(en["users.teamDuplicate"]),
    );
    expect(field).toHaveAttribute("aria-invalid", "true");
    expect(dialog.queryByText(TAKEN.detail)).toBeNull();
    await user.type(field, "s");
    expect(dialog.queryByText(en["users.teamDuplicate"])).toBeNull();
  },
);
