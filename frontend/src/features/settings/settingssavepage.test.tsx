// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { UnsavedGuard } from "../../components/unsaved";
import { useSaveDraft } from "../../design-system/savepage";
import { LocaleProvider } from "../../i18n";
import { SettingsSavePage } from "./settingssavepage";

afterEach(cleanup);

function NameCard({ write }: Readonly<{ write: (name: string) => void }>) {
  const [saved, setSaved] = useState("");
  const [draft, setDraft] = useState("");
  useSaveDraft({
    dirty: draft !== saved,
    save: async () => {
      write(draft);
      setSaved(draft);
    },
    discard: () => setDraft(saved),
  });
  return (
    <input
      aria-label="Name"
      value={draft}
      onChange={(event) => setDraft(event.target.value)}
    />
  );
}

function Shell({ write }: Readonly<{ write: (name: string) => void }>) {
  const [address, setAddress] = useState("account");
  return (
    <LocaleProvider initial="en">
      <button type="button" onClick={() => setAddress("contacts")}>
        Contacts
      </button>
      <UnsavedGuard address={address} onKeep={setAddress}>
        {(shown) =>
          shown === "account" ? (
            <SettingsSavePage>
              <NameCard write={write} />
            </SettingsSavePage>
          ) : (
            <p>Contacts page</p>
          )
        }
      </UnsavedGuard>
    </LocaleProvider>
  );
}

it("asks on the way out of a settings draft, and Save writes it before leaving", async () => {
  const user = userEvent.setup();
  const write = vi.fn();
  render(<Shell write={write} />);
  await user.type(screen.getByRole("textbox", { name: "Name" }), "Jane");
  await user.click(screen.getByRole("button", { name: "Contacts" }));

  const dialog = screen.getByRole("dialog");
  expect(dialog).toHaveTextContent("Save your changes?");
  await user.click(within(dialog).getByRole("button", { name: "Save" }));

  expect(write).toHaveBeenCalledWith("Jane");
  expect(await screen.findByText("Contacts page")).toBeInTheDocument();
});

it("asks the window before a reload only while the page holds a draft", async () => {
  const user = userEvent.setup();
  render(<Shell write={vi.fn()} />);
  const clean = new Event("beforeunload", { cancelable: true });
  globalThis.dispatchEvent(clean);
  expect(clean.defaultPrevented).toBe(false);

  await user.type(screen.getByRole("textbox", { name: "Name" }), "Jane");
  const dirty = new Event("beforeunload", { cancelable: true });
  globalThis.dispatchEvent(dirty);
  expect(dirty.defaultPrevented).toBe(true);
});

function OfferedCard({ write }: Readonly<{ write: () => void }>) {
  useSaveDraft({
    dirty: false,
    offered: true,
    save: async () => write(),
    discard: () => undefined,
  });
  return <p>Defaults offered</p>;
}

it("offers a default through the bar without asking on the way out", async () => {
  const user = userEvent.setup();
  const write = vi.fn();
  render(
    <LocaleProvider initial="en">
      <SettingsSavePage>
        <OfferedCard write={write} />
      </SettingsSavePage>
    </LocaleProvider>,
  );
  const reload = new Event("beforeunload", { cancelable: true });
  globalThis.dispatchEvent(reload);
  expect(reload.defaultPrevented).toBe(false);

  await user.click(screen.getByRole("button", { name: "Save" }));
  expect(write).toHaveBeenCalledTimes(1);
});
