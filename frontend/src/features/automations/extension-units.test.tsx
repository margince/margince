/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ExtensionDescriptor } from "@composition/extensions";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { ExtensionUnitsCard } from "./extension-units";

// The card reads the COMPOSED registry, which is empty in this lane and in
// every core developer's checkout — so the populated case is reached by
// standing in for that module. The alternative, threading a registry prop
// through the card purely so a test could pass one, would put a seam in
// production code that only tests use.
//
// lib/extensions.ts is mocked rather than @composition/extensions, because the
// alias resolves to the committed empty stub whose emptiness is itself pinned
// (extensions.test.ts). Mocking the module under it would make this suite pass
// while the real filter was never called.
const { unitsForSecretScope } = vi.hoisted(() => ({
  unitsForSecretScope:
    vi.fn<(scope: "user" | "workspace") => readonly ExtensionDescriptor[]>(),
}));

vi.mock("../../lib/extensions", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../lib/extensions")>()),
  unitsForSecretScope,
}));

// A unit's copy as gen-composition merges it, so a title resolves as it would composed.
vi.mock("@composition/copy", () => ({
  extensionCopy: { en: { "extDispactConnector.title": "Dispact" } },
}));

const DISPACT: ExtensionDescriptor = {
  name: "dispact-connector",
  title: "extDispactConnector.title",
  secretScope: "user",
  panels: [],
  verbs: [],
  entries: [],
};

function renderCard(scope: "user" | "workspace") {
  return render(
    <LocaleProvider>
      <ExtensionUnitsCard scope={scope} />
    </LocaleProvider>,
  );
}

afterEach(() => {
  cleanup();
  unitsForSecretScope.mockReset();
});

describe("the units offered on a settings page", () => {
  it("names the unit by its title alone and opens its own screen", () => {
    unitsForSecretScope.mockReturnValue([DISPACT]);
    renderCard("user");
    // The row's chevron is the keyboard path, named with the unit it opens.
    fireEvent.click(
      screen.getByRole("button", { name: "Settings for Dispact" }),
    );
    expect(globalThis.location.hash).toBe("#/ext/dispact-connector");
    // The name is visible text, not only part of the chevron's accessible name.
    expect(screen.getByTestId("ext-unit-dispact-connector").textContent).toBe(
      "Dispact",
    );
  });

  it("opens the unit from a press anywhere on its row", () => {
    unitsForSecretScope.mockReturnValue([DISPACT]);
    renderCard("user");
    globalThis.location.hash = "";
    fireEvent.click(screen.getByText("Dispact"));
    expect(globalThis.location.hash).toBe("#/ext/dispact-connector");
  });

  it("reads a unit with no title by its key, as the extensions page does", () => {
    unitsForSecretScope.mockReturnValue([
      {
        name: "open_notes",
        title: "",
        secretScope: "user",
        panels: [],
        verbs: [],
        entries: [],
      },
    ]);
    renderCard("user");
    expect(
      screen.getByRole("button", { name: "Settings for Open notes" }),
    ).toBeTruthy();
    expect(screen.getByTestId("ext-unit-open_notes").textContent).toBe(
      "Open notes",
    );
  });

  it("asks for the scope of the page it was placed on", () => {
    unitsForSecretScope.mockReturnValue([]);
    renderCard("workspace");
    expect(unitsForSecretScope).toHaveBeenCalledWith("workspace");
  });

  // A heading over an empty list is a promise the installation did not make —
  // and it is the state of every vanilla checkout, so it is the common case
  // rather than an edge one.
  it("renders nothing at all when no unit is offered", () => {
    unitsForSecretScope.mockReturnValue([]);
    const { container } = renderCard("user");
    expect(container.innerHTML).toBe("");
  });

  it("gives every offered unit its own row", () => {
    unitsForSecretScope.mockReturnValue([
      DISPACT,
      {
        name: "notes",
        title: "",
        secretScope: "user",
        verbs: [],
        entries: [],
        panels: [],
      },
    ]);
    renderCard("user");
    expect(screen.getAllByRole("button")).toHaveLength(2);
    fireEvent.click(screen.getByRole("button", { name: "Settings for Notes" }));
    expect(globalThis.location.hash).toBe("#/ext/notes");
  });
});
