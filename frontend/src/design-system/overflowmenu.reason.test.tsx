/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { Button } from "./atoms";
import { OverflowMenu } from "./overflowmenu";

afterEach(cleanup);

const REFUSAL = "You cannot edit this company.";

function RecordHeader({ refused }: Readonly<{ refused: boolean }>) {
  return (
    <>
      <p id="page-reason">{REFUSAL}</p>
      <OverflowMenu label="More actions">
        <Button reasonId={refused ? "page-reason" : undefined}>Merge</Button>
        <Button>Full history</Button>
      </OverflowMenu>
    </>
  );
}

async function openPanel() {
  await userEvent.click(screen.getByRole("button", { name: "More actions" }));
  return screen.getByRole("button", { name: "Merge" })
    .parentElement as HTMLElement;
}

describe("OverflowMenu — why its items are refused", () => {
  it("draws the page's refusal inside the open panel", async () => {
    render(<RecordHeader refused />);
    const panel = await openPanel();
    expect(within(panel).getByText(REFUSAL)).toBeVisible();
  });

  it("says nothing when no item is refused", async () => {
    render(<RecordHeader refused={false} />);
    const panel = await openPanel();
    expect(within(panel).queryByText(REFUSAL)).toBeNull();
  });

  it("does not repeat a sentence the panel already holds", async () => {
    render(
      <OverflowMenu label="More actions">
        <p id="own-reason">{REFUSAL}</p>
        <Button reasonId="own-reason">Merge</Button>
      </OverflowMenu>,
    );
    const panel = await openPanel();
    expect(within(panel).getAllByText(REFUSAL)).toHaveLength(1);
  });

  it("adds the reason when an item turns refused while the panel is open", async () => {
    function Late() {
      const [refused, setRefused] = useState(false);
      return (
        <>
          <p id="page-reason">{REFUSAL}</p>
          <OverflowMenu label="More actions">
            <Button reasonId={refused ? "page-reason" : undefined}>
              Merge
            </Button>
            <Button onClick={() => setRefused(true)} aria-pressed={refused}>
              Refuse
            </Button>
          </OverflowMenu>
        </>
      );
    }
    render(<Late />);
    const panel = await openPanel();
    await userEvent.click(
      within(panel).getByRole("button", { name: "Refuse" }),
    );
    expect(await within(panel).findByText(REFUSAL)).toBeVisible();
  });
});
