/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { en as englishCopy } from "../../i18n/en";
import {
  renderSettings,
  settingsBackend,
} from "../../testing/settings.testkit";

beforeEach(() => {
  vi.stubGlobal("fetch", settingsBackend());
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The reader's own agent switches stand in view; Agent rules has an address.
describe("AI assistants: Your agent and Agent rules", () => {
  it("keeps the reader's own agent switches in view, outside For developers", async () => {
    renderSettings("agents");
    const yours = await screen.findByRole("heading", {
      level: 2,
      name: "Your agent",
    });
    const developers = screen
      .getByText(englishCopy["aiAccess.dev.title"])
      .closest("details");
    for (const card of ["Automatic changes", "Personal approval mutes"]) {
      const title = screen.getByRole("heading", { level: 3, name: card });
      expect(developers?.contains(title)).toBe(false);
      expect(
        yours.compareDocumentPosition(title) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy();
    }
    expect(developers?.open).toBe(false);
  });

  it("opens Agent rules from its own address", async () => {
    renderSettings("agents", "rules");
    expect(
      await screen.findByRole("heading", {
        name: englishCopy["settings.autonomy"],
      }),
    ).toBeVisible();
    const developers = screen
      .getByText(englishCopy["aiAccess.dev.title"])
      .closest("details");
    expect(developers?.open).toBe(true);
    expect(
      screen.getByRole("button", { name: "Agent rules", pressed: true }),
    ).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Passports" })).toBeNull();
  });
});
