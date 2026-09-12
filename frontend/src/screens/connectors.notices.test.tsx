// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { OAuthOutcomeNote } from "./connectors.notices";

// The outcome segment arrives on the address bar, so it is reader-typed rather
// than server-chosen, and the table it indexes is a plain object literal. On
// one of those `constructor`, `toString` and their siblings are INHERITED
// members: a bare index answers them with a function instead of undefined, and
// the note then renders with no tone and no sentence — a callout that says
// nothing, sitting in the one place a reader looks to find out what happened.
//
// The Storybook frame beside this file draws the ordinary miss, but a story
// carries no assertion and vitest does not collect one, so the guard that makes
// the miss real needs a test that fails when the guard goes.

function mount(segment: string) {
  globalThis.location.hash = `#/settings/connections/${segment}`;
  return render(
    <LocaleProvider initial="en">
      <OAuthOutcomeNote />
    </LocaleProvider>,
  );
}

afterEach(cleanup);

describe("the OAuth return note", () => {
  it("reports an outcome the server defines", () => {
    mount("ok");
    expect(screen.getByText("Connection made")).toBeTruthy();
  });

  it.each([
    "constructor",
    "toString",
    "__proto__",
    "valueOf",
    "hasOwnProperty",
  ])("draws nothing for the inherited member %s", (segment) => {
    const { container } = mount(segment);
    expect(container.firstChild).toBeNull();
  });

  it("draws nothing for a segment this build simply does not know", () => {
    const { container } = mount("unrecognised");
    expect(container.firstChild).toBeNull();
  });
});
