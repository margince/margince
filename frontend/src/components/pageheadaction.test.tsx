// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import {
  PageHeadAction,
  PageHeadActionProvider,
  PageHeadActionSlot,
} from "./pageheadaction";

afterEach(cleanup);

// The shell's shape: the head with its slot, and the page under it.
function Page({ verb }: Readonly<{ verb: boolean }>) {
  return (
    <PageHeadActionProvider>
      <header data-testid="head">
        <PageHeadActionSlot />
      </header>
      <main data-testid="page">
        {verb && (
          <PageHeadAction>
            <button type="button">Invite member</button>
          </PageHeadAction>
        )}
      </main>
    </PageHeadActionProvider>
  );
}

describe("PageHeadAction", () => {
  it("draws the page's verb in the head, not in the page that declares it", async () => {
    render(<Page verb />);
    const head = screen.getByTestId("head");
    expect(
      await within(head).findByRole("button", { name: "Invite member" }),
    ).toBeTruthy();
    expect(within(screen.getByTestId("page")).queryByRole("button")).toBeNull();
  });

  it("leaves the head's slot empty while the page withholds its verb", () => {
    render(<Page verb={false} />);
    const slot = screen.getByTestId("head").querySelector(".pagehead-action");
    expect(slot?.childElementCount).toBe(0);
  });

  it("draws the verb where it is written when no head stands around it", () => {
    render(
      <PageHeadAction>
        <button type="button">New team</button>
      </PageHeadAction>,
    );
    const verb = screen.getByRole("button", { name: "New team" });
    expect(verb.parentElement?.className).toBe("pagehead-action");
  });
});
