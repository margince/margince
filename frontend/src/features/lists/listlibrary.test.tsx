// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { en } from "../../i18n/en";
import { listsMe, TEAM_ID, teamsPage } from "../../testing/lists.fixtures";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { NewShortlistAction } from "./listlibrary";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("New Shortlist", () => {
  it("starts every opening from nothing typed before, its audience included", async () => {
    installFetchStub({
      "GET /me": listsMe(true, [TEAM_ID]),
      "GET /teams": () => jsonResponse(teamsPage),
    });
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <NewShortlistAction />
      </StoryProviders>,
    );
    const start = await screen.findByRole("button", {
      name: en["lists.newShortlist"],
    });
    await user.click(start);
    await user.type(
      screen.getByRole("textbox", { name: en["lists.name"] }),
      "Dinner",
    );
    await user.type(
      screen.getByRole("textbox", { name: en["lists.purpose"] }),
      "October",
    );
    await user.click(
      screen.getByRole("combobox", { name: en["lists.recordTypeLabel"] }),
    );
    await user.click(
      await screen.findByRole("option", { name: en["lists.type.company"] }),
    );
    await user.click(
      screen.getByRole("combobox", { name: en["lists.sharingLabel"] }),
    );
    await user.click(
      await screen.findByRole("option", { name: en["lists.sharing.private"] }),
    );
    await user.click(screen.getByRole("button", { name: en["create.cancel"] }));

    await user.click(start);
    expect(
      await screen.findByRole("textbox", { name: en["lists.name"] }),
    ).toHaveValue("");
    expect(
      screen.getByRole("textbox", { name: en["lists.purpose"] }),
    ).toHaveValue("");
    expect(
      screen.getByRole("combobox", { name: en["lists.recordTypeLabel"] }),
    ).toHaveTextContent(en["lists.type.contact"]);
    expect(
      screen.getByRole("combobox", { name: en["lists.sharingLabel"] }),
    ).toHaveTextContent(en["lists.sharing.team"]);
  });
});
