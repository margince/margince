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
  expect(described?.querySelector("code")).toHaveTextContent(
    "deal.cf_fleet_size",
  );
});
