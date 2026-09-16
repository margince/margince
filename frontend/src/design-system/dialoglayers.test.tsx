/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { Modal } from "./atoms";
import { Heading } from "./heading";
import { Popover } from "./popover";

afterEach(cleanup);

function Layers({ close }: Readonly<{ close: () => void }>) {
  const [reading, setReading] = useState(false);
  return (
    <Modal open onClose={close} labelledBy="composer-title">
      <Heading size="large" id="composer-title">
        Composer
      </Heading>
      <button type="button" onClick={() => setReading(true)}>
        Read email
      </button>
      <Popover label="Preview">The whole message.</Popover>
      <button type="button">Send</button>
      <Modal
        open={reading}
        onClose={() => setReading(false)}
        labelledBy="reader-title"
      >
        <Heading size="large" id="reader-title">
          Email reader
        </Heading>
        <button type="button" onClick={() => setReading(false)}>
          Close reader
        </button>
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
