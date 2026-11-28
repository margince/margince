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
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider, translate } from "../i18n";
import { en as englishCopy } from "../i18n/en";
import { ProblemError } from "../lib/problem";
import { NameDialog, nameRefusal } from "./namedialog";

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

it("describes the name field by its hint line", async () => {
  render(
    <LocaleProvider initial="en">
      <NameDialog
        open
        onClose={() => undefined}
        title="Rename field"
        label="Field name"
        initial="Fleet size"
        hint="Existing records keep the old name."
        confirmLabel="Save"
        onSave={() => undefined}
      />
    </LocaleProvider>,
  );

  const field = await screen.findByRole("textbox", { name: "Field name" });
  const described = document.getElementById(
    field.getAttribute("aria-describedby") ?? "",
  );
  expect(described).toHaveTextContent("Existing records keep the old name.");
  expect(described?.querySelector("code")).toBeNull();
});

function Name({
  onClose,
  onDirtyChange,
}: Readonly<{
  onClose: () => void;
  onDirtyChange?: (dirty: boolean) => void;
}>) {
  return (
    <LocaleProvider initial="en">
      <NameDialog
        open
        onClose={onClose}
        onDirtyChange={onDirtyChange}
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
    screen.getByRole("dialog", { name: englishCopy["unsaved.title"] }),
  ).toBeInTheDocument();
  expect(onClose).not.toHaveBeenCalled();
});

it("reports a changed name as unsaved until it reads as it started", async () => {
  const onDirtyChange = vi.fn();
  const user = userEvent.setup({ delay: null });
  render(<Name onClose={() => undefined} onDirtyChange={onDirtyChange} />);
  const field = screen.getByRole("textbox", { name: "Name" });
  await user.type(field, "er");
  expect(onDirtyChange).toHaveBeenLastCalledWith(true);
  await user.type(field, "{Backspace}{Backspace} ");
  expect(onDirtyChange).toHaveBeenLastCalledWith(false);
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

it("says an empty name is required on Save, focuses it, and writes nothing", async () => {
  const onSave = vi.fn();
  const user = userEvent.setup({ delay: null });
  render(
    <LocaleProvider initial="en">
      <NameDialog
        open
        onClose={() => undefined}
        title="New view"
        label="View name"
        confirmLabel="Create"
        onSave={onSave}
      />
    </LocaleProvider>,
  );

  await user.click(screen.getByRole("button", { name: "Create" }));

  expect(screen.getByText("View name is required.")).toBeVisible();
  expect(screen.getByRole("textbox", { name: /View name/ })).toHaveFocus();
  expect(onSave).not.toHaveBeenCalled();
});

it("takes no more characters than its limit", async () => {
  const user = userEvent.setup({ delay: null });
  render(
    <LocaleProvider initial="en">
      <NameDialog
        open
        onClose={() => undefined}
        title="New team"
        label="Team name"
        confirmLabel="Create"
        maxLength={5}
        onSave={() => undefined}
      />
    </LocaleProvider>,
  );
  const field = screen.getByRole("textbox", { name: "Team name" });

  await user.type(field, "Operations");

  expect(field).toHaveValue("Opera");
});

function Titled({ title, label }: Readonly<{ title: string; label: string }>) {
  return (
    <LocaleProvider initial="en">
      <NameDialog
        open
        onClose={() => undefined}
        title={title}
        label={label}
        initial="Berlin"
        confirmLabel="Save"
        onSave={() => undefined}
      />
    </LocaleProvider>
  );
}

it("prints a label that repeats the title once and keeps it as the field name", () => {
  render(<Titled title="Deal name" label="Deal name" />);
  const printed = screen
    .getAllByText("Deal name")
    .filter((node) => !node.classList.contains("sr-only"));
  expect(printed).toHaveLength(1);
  expect(screen.getByRole("textbox", { name: "Deal name" })).toBeVisible();
});

it("prints both when the label differs from the title", () => {
  render(<Titled title="Rename deal" label="Deal name" />);
  expect(screen.getByText("Rename deal")).toBeVisible();
  expect(screen.getByText("Deal name")).not.toHaveClass("sr-only");
});

describe("nameRefusal", () => {
  const t = (key: Parameters<typeof translate>[1]) => translate("en", key);
  const team = {
    label: "Team name",
    duplicate: "users.teamDuplicate",
  } as const;

  it("puts a taken name on the name, in the caller's words", () => {
    const taken = new ProblemError({
      status: 409,
      code: "name_taken",
      detail: 'a team called "Ops" already exists',
    });
    expect(nameRefusal(taken, t, team)).toEqual({
      problem: null,
      nameProblem: t("users.teamDuplicate"),
    });
  });

  it("leaves a plain conflict under the form, never claiming the name is taken", () => {
    const stale = new ProblemError({
      status: 409,
      code: "conflict",
      detail: "The team changed since you opened it. Reload and try again.",
    });
    expect(nameRefusal(stale, t, team)).toEqual({
      problem: "The team changed since you opened it. Reload and try again.",
      nameProblem: null,
    });
  });

  it("puts only the caller's own code on the name when it names one", () => {
    const taken = new ProblemError({
      status: 409,
      code: "custom_field_label_taken",
      detail: "a custom field with this label already exists",
    });
    const busy = new ProblemError({
      status: 409,
      code: "conflict",
      detail: "The table is busy. Try again.",
    });
    const field = {
      label: "Label",
      duplicate: "cf.labelTaken",
      code: "custom_field_label_taken",
    } as const;
    expect(nameRefusal(taken, t, field)).toEqual({
      problem: null,
      nameProblem: t("cf.labelTaken"),
    });
    expect(nameRefusal(busy, t, field)).toEqual({
      problem: "The table is busy. Try again.",
      nameProblem: null,
    });
  });

  it.each(["name", "label"])(
    "puts a too-long %s on the name, worded with the field and its limit",
    (field) => {
      const tooLong = new ProblemError({
        status: 422,
        code: "validation_error",
        detail: "the request failed validation",
        details: {
          errors: [{ field, code: "too_long", message: "too long", max: 120 }],
        },
      });
      expect(nameRefusal(tooLong, t, team)).toEqual({
        problem: null,
        nameProblem: "Team name: at most 120 characters.",
      });
    },
  );

  it("keeps any other refusal under the form", () => {
    const denied = new ProblemError({
      status: 403,
      code: "permission_denied",
      detail: "x",
    });
    expect(nameRefusal(denied, t, team)).toEqual({
      problem: t("common.permissionDenied"),
      nameProblem: null,
    });
  });

  it("says nothing before a write is refused", () => {
    expect(nameRefusal(null, t, team)).toEqual({
      problem: null,
      nameProblem: null,
    });
  });
});
