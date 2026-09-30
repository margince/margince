/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Button, Modal } from "./atoms";
import { ConfirmModal } from "./confirmmodal";
import { Heading } from "./heading";
import { holdExits } from "./presence-testing";

// A confirmation raised over a drawer stays painted while its exit plays. The
// keyboard is the drawer's from the moment the reader dismisses it, not from
// the moment the animation ends.

afterEach(cleanup);

// An exit that never ends, so every assertion below runs mid-exit.
beforeEach(() => {
  const exits = holdExits();
  return () => exits.mockRestore();
});

function DrawerWithConfirm({ onClose }: Readonly<{ onClose: () => void }>) {
  const [confirming, setConfirming] = useState(false);
  return (
    <Modal open onClose={onClose} labelledBy="links" placement="right">
      <Heading size="large" id="links">
        Shared links
      </Heading>
      <Button onClick={() => setConfirming(true)}>End link</Button>
      <Button>Copy link</Button>
      <ConfirmModal
        open={confirming}
        onClose={() => setConfirming(false)}
        title="End this link?"
        confirmLabel="End"
        onConfirm={() => setConfirming(false)}
      >
        <p>Anyone holding it loses access.</p>
      </ConfirmModal>
    </Modal>
  );
}

/** Opens the confirmation and dismisses it, leaving it mid-exit. */
async function dismissConfirmation() {
  const onClose = vi.fn();
  const user = userEvent.setup();
  render(<DrawerWithConfirm onClose={onClose} />);
  await user.click(screen.getByRole("button", { name: "End link" }));
  expect(screen.getByRole("dialog", { name: "End this link?" })).toBeTruthy();
  await user.keyboard("{Escape}");

  const leaving = document.querySelectorAll(".overlay")[1];
  expect(leaving?.getAttribute("data-state")).toBe("closing");
  expect(leaving?.hasAttribute("inert")).toBe(true);
  expect(onClose).not.toHaveBeenCalled();
  return { onClose, user };
}

describe("a dialog on its way out hands the keyboard to the one below", () => {
  it("lets Escape close the drawer while the confirmation is still leaving", async () => {
    const { onClose, user } = await dismissConfirmation();
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("keeps Tab inside the drawer while the confirmation is still leaving", async () => {
    const { user } = await dismissConfirmation();
    const drawer = screen.getByRole("dialog", { name: "Shared links" });
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "End link" }),
    );
    await user.keyboard("{Shift>}{Tab}{/Shift}");
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Close" }),
    );
    expect(drawer.contains(document.activeElement)).toBe(true);
  });
});
