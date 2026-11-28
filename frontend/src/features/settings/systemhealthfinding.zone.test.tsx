// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { composeStories } from "@storybook/react-vite";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { en as englishCopy } from "../../i18n/en";
import * as stories from "./systemhealthfinding.stories";

const { ZoneUnresolved } = composeStories(stories);

afterEach(() => {
  cleanup();
  window.location.hash = "";
});

describe("the unloadable-zone finding panel", () => {
  it("opens the settings part that holds the zone", async () => {
    render(<ZoneUnresolved />);
    await userEvent.click(
      await screen.findByRole("button", {
        name: englishCopy["health.openRegional"],
      }),
    );
    expect(window.location.hash).toBe("#/settings/company/regional");
  });
});
