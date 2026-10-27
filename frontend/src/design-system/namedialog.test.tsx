/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import {
  QueryClient,
  QueryClientProvider,
  useMutation,
} from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { KeyCode } from "./keyedname";
import { NameDialog } from "./namedialog";

afterEach(cleanup);

// A real mutation, because its `isPending` reaches the dialog one task after
// the press, which is the gap a second Enter lands in.
function Rename({ write }: Readonly<{ write: (name: string) => void }>) {
  const save = useMutation({
    mutationFn: async (name: string) => {
      write(name);
      return new Promise<never>(() => undefined);
    },
  });
  return (
    <NameDialog
      open
      onClose={() => undefined}
      title="Rename"
      label="Name"
      confirmLabel="Save"
      pending={save.isPending}
      onSave={(name) => save.mutate(name)}
    />
  );
}

it("sends one write for a double Enter", async () => {
  const write = vi.fn();
  const user = userEvent.setup({ delay: null });
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <Rename write={write} />
      </LocaleProvider>
    </QueryClientProvider>,
  );

  await user.type(
    await screen.findByRole("textbox", { name: "Name" }),
    "Berlin{Enter}{Enter}",
  );

  await vi.waitFor(() => expect(write).toHaveBeenCalled());
  expect(write).toHaveBeenCalledExactlyOnceWith("Berlin");
});

it("describes the name field by its hint, the key in the code face", async () => {
  render(
    <LocaleProvider initial="en">
      <NameDialog
        open
        onClose={() => undefined}
        title="Rename field"
        label="Field name"
        initial="Fleet size"
        hint={<KeyCode code="deal.cf_fleet_size" />}
        confirmLabel="Save"
        onSave={() => undefined}
      />
    </LocaleProvider>,
  );

  const field = await screen.findByRole("textbox", { name: "Field name" });
  const described = document.getElementById(
    field.getAttribute("aria-describedby") ?? "",
  );
  expect(described).toHaveTextContent("Key: deal.cf_fleet_size");
  expect(described?.querySelector("code")).toHaveTextContent(
    "deal.cf_fleet_size",
  );
});

function Name({ onClose }: Readonly<{ onClose: () => void }>) {
  return (
    <LocaleProvider initial="en">
      <NameDialog
        open
        onClose={onClose}
        title="Rename"
        label="Name"
        initial="Berlin"
        confirmLabel="Save"
        onSave={() => undefined}
      />
    </LocaleProvider>
  );
}

it("asks before Escape drops a changed name", async () => {
  const onClose = vi.fn();
  const user = userEvent.setup({ delay: null });
  render(<Name onClose={onClose} />);
  await user.type(screen.getByRole("textbox", { name: "Name" }), " Mitte");
  await user.keyboard("{Escape}");
  expect(
    screen.getByRole("dialog", { name: "Discard unsaved changes?" }),
  ).toBeInTheDocument();
  expect(onClose).not.toHaveBeenCalled();
});

it("closes on Escape at once while the name is unchanged", async () => {
  const onClose = vi.fn();
  const user = userEvent.setup({ delay: null });
  render(<Name onClose={onClose} />);
  await user.keyboard("{Escape}");
  expect(onClose).toHaveBeenCalledOnce();
});

// The list editors seed a rename with the label in the reader's language. An
// unchanged save stores nothing, and a rename sends what was typed.
it("writes nothing for an unchanged name and sends the typed one", async () => {
  const onSave = vi.fn();
  const user = userEvent.setup({ delay: null });
  render(
    <LocaleProvider initial="de">
      <NameDialog
        open
        onClose={() => undefined}
        title="Umbenennen"
        label="Name"
        initial="Eingehend"
        confirmLabel="Speichern"
        onSave={onSave}
      />
    </LocaleProvider>,
  );
  const field = await screen.findByRole("textbox", { name: "Name" });

  await user.type(field, " {Enter}");
  await user.click(screen.getByRole("button", { name: "Speichern" }));
  expect(onSave).not.toHaveBeenCalled();

  await user.clear(field);
  await user.type(field, "Eingehende Anfragen{Enter}");
  expect(onSave).toHaveBeenCalledExactlyOnceWith("Eingehende Anfragen");
});
