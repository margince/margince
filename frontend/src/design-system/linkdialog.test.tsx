/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { LinkDialog } from "./linkdialog";
import { linkHref } from "./richtext-html";

afterEach(cleanup);

function open(href = "", onRemove?: () => void) {
  const onApply = vi.fn();
  const onClose = vi.fn();
  render(
    <LocaleProvider initial="en">
      <LinkDialog
        open
        href={href}
        onApply={onApply}
        onRemove={onRemove}
        onClose={onClose}
      />
    </LocaleProvider>,
  );
  return { onApply, onClose };
}

describe("the address a typed link stands for", () => {
  it("gives a bare host https and keeps a typed scheme", () => {
    expect(linkHref("example.com/no-scheme")).toBe(
      "https://example.com/no-scheme",
    );
    expect(linkHref("www.x.de")).toBe("https://www.x.de");
    expect(linkHref("example.com?view=full")).toBe(
      "https://example.com?view=full",
    );
    expect(linkHref("example.com:8443/x")).toBe("https://example.com:8443/x");
    expect(linkHref(" http://example.com ")).toBe("http://example.com");
    expect(linkHref("mailto:sofia@example.com")).toBe(
      "mailto:sofia@example.com",
    );
  });

  it("writes to a bare mailbox", () => {
    expect(linkHref("sofia@example.com")).toBe("mailto:sofia@example.com");
  });

  it("refuses every other scheme and an empty address", () => {
    for (const typed of [
      "javascript:alert(1)",
      "data:text/html,<b>x</b>",
      "   ",
      "https://",
      "mailto:",
      "mailto:sofia@example.com?cc=boss@example.com",
      "mailto:sofia@example.com%0ABcc:boss@example.com",
      "/relative/path",
      "just words",
    ]) {
      expect(linkHref(typed), typed).toBe("");
    }
  });
});

describe("the link dialog", () => {
  it("applies the address it was given, with a scheme", async () => {
    const user = userEvent.setup({ delay: null });
    const { onApply } = open();

    await user.type(
      screen.getByRole("textbox", { name: "Web or email address" }),
      "example.com/x{Enter}",
    );

    expect(onApply).toHaveBeenCalledExactlyOnceWith("https://example.com/x");
  });

  it("refuses a script address on the field and keeps refusing", async () => {
    const user = userEvent.setup({ delay: null });
    const { onApply } = open();
    const field = screen.getByRole("textbox", { name: "Web or email address" });

    await user.type(field, "javascript:alert(1){Enter}");
    await user.click(screen.getByRole("button", { name: "Apply" }));

    expect(onApply).not.toHaveBeenCalled();
    expect(field).toHaveAttribute("aria-invalid", "true");
    expect(
      screen.getByText(/This is not a web or email address/),
    ).toBeInTheDocument();
  });

  it("offers Remove link only for a link that is there", async () => {
    const user = userEvent.setup({ delay: null });
    const onRemove = vi.fn();
    open("https://example.com", onRemove);

    expect(
      screen.getByRole("textbox", { name: "Web or email address" }),
    ).toHaveValue("https://example.com");
    expect(screen.getByRole("heading", { name: "Edit link" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Remove link" }));
    expect(onRemove).toHaveBeenCalledOnce();

    cleanup();
    open();
    expect(screen.getByRole("heading", { name: "Add link" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "Remove link" })).toBeNull();
  });

  it("closes on Escape without applying", async () => {
    const user = userEvent.setup({ delay: null });
    const { onApply, onClose } = open();

    await user.keyboard("{Escape}");

    expect(onClose).toHaveBeenCalled();
    expect(onApply).not.toHaveBeenCalled();
  });
});
