/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { Button, Modal } from "./atoms";
import { Popover } from "./popover";

afterEach(cleanup);

function Layers({ close }: Readonly<{ close: () => void }>) {
  const [reading, setReading] = useState(false);
  return (
    <Modal open onClose={close} labelledBy="composer-title">
      <h2 id="composer-title">Composer</h2>
      <Button onClick={() => setReading(true)}>Read email</Button>
      <Popover label="Preview">The whole message.</Popover>
      <Button>Send</Button>
      <Modal
        open={reading}
        onClose={() => setReading(false)}
        labelledBy="reader-title"
      >
        <h2 id="reader-title">Email reader</h2>
        <Button onClick={() => setReading(false)}>Close reader</Button>
        <a href="#attachment">Attachment</a>
      </Modal>
    </Modal>
  );
}

it("traps Tab in the reader and lets Escape close only the reader", async () => {
  const user = userEvent.setup();
  const close = vi.fn();
  render(<Layers close={close} />);
  await user.click(screen.getByRole("button", { name: "Read email" }));
  expect(document.activeElement).toBe(
    screen.getByRole("button", { name: "Close reader" }),
  );
  await user.tab();
  expect(document.activeElement).toBe(
    screen.getByRole("link", { name: "Attachment" }),
  );
  // Every dialog draws its own way out as its last stop, and the composer
  // underneath draws one too — so the stop Tab finds here has to be the
  // READER's, not the one on the layer below it.
  await user.tab();
  const reader = screen.getByRole("dialog", { name: "Email reader" });
  expect(document.activeElement?.getAttribute("aria-label")).toBe("Close");
  expect(reader.contains(document.activeElement)).toBe(true);
  await user.tab();
  expect(document.activeElement).toBe(
    screen.getByRole("button", { name: "Close reader" }),
  );
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("dialog", { name: "Email reader" })).toBeNull();
  expect(close).not.toHaveBeenCalled();
  expect(document.activeElement).toBe(
    screen.getByRole("button", { name: "Read email" }),
  );
});

it("dismisses a prose-only preview before its parent dialog", async () => {
  const user = userEvent.setup();
  const close = vi.fn();
  render(<Layers close={close} />);
  await user.click(screen.getByRole("button", { name: "Preview" }));
  await user.keyboard("{Escape}");
  expect(screen.queryByText("The whole message.")).toBeNull();
  expect(close).not.toHaveBeenCalled();
  await user.keyboard("{Escape}");
  expect(close).toHaveBeenCalledTimes(1);
});
