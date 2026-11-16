/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { Button } from "./atoms";
import { ListPopover } from "./listpopover";
import { OverflowMenu, useCloseOverflowMenu } from "./overflowmenu";

afterEach(cleanup);

function MoveToStage({ onMove }: Readonly<{ onMove: (id: string) => void }>) {
  const closeMenu = useCloseOverflowMenu();
  return (
    <ListPopover
      label="Move to stage"
      title="Move to stage"
      searchLabel="Pick a stage"
      options={[
        { id: "proposal", name: "Proposal" },
        { id: "won", name: "Won" },
      ]}
      onPick={(stage, done) => {
        done();
        closeMenu();
        onMove(stage.id);
      }}
    />
  );
}

it("stays open over a list an item opened, and closes on a pick in it", async () => {
  const user = userEvent.setup();
  const onMove = vi.fn();
  render(
    <OverflowMenu label="More actions">
      <MoveToStage onMove={onMove} />
      <Button>Archive</Button>
    </OverflowMenu>,
  );
  const trigger = screen.getByRole("button", { name: "More actions" });
  await user.click(trigger);
  await user.click(screen.getByRole("button", { name: "Move to stage" }));
  expect(trigger).toHaveAttribute("aria-expanded", "true");

  await user.click(await screen.findByRole("option", { name: "Won" }));
  expect(onMove).toHaveBeenCalledWith("won");
  expect(trigger).toHaveAttribute("aria-expanded", "false");
  expect(trigger).toHaveFocus();
});

it("closes nothing outside a menu", async () => {
  const user = userEvent.setup();
  const onMove = vi.fn();
  render(<MoveToStage onMove={onMove} />);
  await user.click(screen.getByRole("button", { name: "Move to stage" }));
  await user.click(await screen.findByRole("option", { name: "Proposal" }));
  expect(onMove).toHaveBeenCalledWith("proposal");
});
