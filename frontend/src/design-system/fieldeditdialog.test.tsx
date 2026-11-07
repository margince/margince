/** @vitest-environment happy-dom */
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@testing-library/jest-dom/vitest";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { ProblemError } from "../lib/problem";
import { Textarea, TextInput } from "./atoms";
import { DateInput, isoOrBlank } from "./dateinput";
import { EditableValue } from "./editablevalue";
import { FieldEditDialog } from "./fieldeditdialog";
import { Select } from "./select";
import { pickOption } from "./select-testing";

afterEach(cleanup);

type Harness = Readonly<{
  onSave?: (next: string) => Promise<void>;
  onDirtyChange?: (dirty: boolean) => void;
  control?: "text" | "textarea" | "choice" | "date";
  stored?: string;
  settle?: (draft: string) => string;
}>;

function Edited({
  onSave = async () => undefined,
  onDirtyChange,
  control = "text",
  settle,
  stored = "Software",
}: Harness) {
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState(stored);
  return (
    <>
      <EditableValue editLabel="Change Industry" onEdit={() => setOpen(true)}>
        {value}
      </EditableValue>
      <FieldEditDialog
        open={open}
        label="Industry"
        value={value}
        settle={settle}
        onDirtyChange={onDirtyChange}
        onClose={() => setOpen(false)}
        onSave={async (next) => {
          await onSave(next);
          setValue(next);
        }}
      >
        {({ control: props, draft, setDraft, saving }) => {
          if (control === "textarea") {
            return (
              <Textarea
                {...props}
                value={draft}
                readOnly={saving}
                onChange={(event) => setDraft(event.target.value)}
              />
            );
          }
          if (control === "date") {
            return (
              <DateInput
                {...props}
                value={isoOrBlank(draft)}
                onChange={(event) => setDraft(event.target.value)}
              />
            );
          }
          if (control === "choice") {
            return (
              <Select
                {...props}
                value={draft}
                options={[
                  { value: "Software", label: "Software" },
                  { value: "Retail", label: "Retail" },
                ]}
                onChange={setDraft}
              />
            );
          }
          return (
            <TextInput
              {...props}
              value={draft}
              readOnly={saving}
              onChange={(event) => setDraft(event.target.value)}
            />
          );
        }}
      </FieldEditDialog>
    </>
  );
}

function renderEdited(props: Harness = {}) {
  render(
    <LocaleProvider initial="en">
      <Edited {...props} />
    </LocaleProvider>,
  );
  return screen.getByRole("button", { name: /Change Industry/ });
}

describe("FieldEditDialog", () => {
  it("opens titled by the field, with focus in the control", async () => {
    const user = userEvent.setup();
    await user.click(renderEdited());
    const dialog = await screen.findByRole("dialog", { name: "Industry" });
    expect(dialog).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByRole("textbox", { name: "Industry" })).toHaveFocus(),
    );
    expect(screen.getByRole("textbox", { name: "Industry" })).toHaveValue(
      "Software",
    );
  });

  it("saves the typed value from Save, closes, and gives focus back", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn(async () => undefined);
    const trigger = renderEdited({ onSave });
    await user.click(trigger);
    const box = await screen.findByRole("textbox", { name: "Industry" });
    await user.clear(box);
    await user.type(box, "Retail");
    await user.click(screen.getByRole("button", { name: en["record.save"] }));
    expect(onSave).toHaveBeenCalledWith("Retail");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(
      screen.getByRole("button", { name: "Change Industry Retail" }),
    ).toHaveFocus();
  });

  it("saves from Enter in a single-line field", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn(async () => undefined);
    await user.click(renderEdited({ onSave }));
    const box = await screen.findByRole("textbox", { name: "Industry" });
    await user.clear(box);
    await user.type(box, "Retail{Enter}");
    expect(onSave).toHaveBeenCalledWith("Retail");
  });

  it("keeps Enter a newline in a paragraph and saves on Ctrl+Enter", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn(async () => undefined);
    await user.click(renderEdited({ onSave, control: "textarea" }));
    const box = await screen.findByRole("textbox", { name: "Industry" });
    await user.type(box, "{Enter}B2B");
    expect(onSave).not.toHaveBeenCalled();
    await user.keyboard("{Control>}{Enter}{/Control}");
    expect(onSave).toHaveBeenCalledWith("Software\nB2B");
  });

  it("saves a day typed into a date field from Enter", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn(async () => undefined);
    await user.click(
      renderEdited({ onSave, control: "date", stored: "2026-12-15" }),
    );
    const box = await screen.findByRole("textbox", { name: "Industry" });
    await user.clear(box);
    await user.type(box, "2026-12-20{Enter}");
    expect(onSave).toHaveBeenCalledWith("2026-12-20");
  });

  it("saves a choice picked in the dialog", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn(async () => undefined);
    await user.click(renderEdited({ onSave, control: "choice" }));
    await pickOption(
      user,
      await screen.findByRole("combobox", { name: "Industry" }),
      "Retail",
    );
    await user.click(screen.getByRole("button", { name: en["record.save"] }));
    expect(onSave).toHaveBeenCalledWith("Retail");
  });

  it("keeps the typed value and shows the refusal on the field", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn(async () => {
      throw new ProblemError({
        code: "version_skew",
        detail: "Somebody else changed this record.",
      });
    });
    await user.click(renderEdited({ onSave }));
    const box = await screen.findByRole("textbox", { name: "Industry" });
    await user.clear(box);
    await user.type(box, "Retail");
    await user.click(screen.getByRole("button", { name: en["record.save"] }));
    await waitFor(() =>
      expect(box).toHaveAccessibleDescription(en["common.versionSkew"]),
    );
    expect(box).toHaveValue("Retail");
    expect(box).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("closes on Escape without saving an untouched value", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn(async () => undefined);
    await user.click(renderEdited({ onSave }));
    await screen.findByRole("textbox", { name: "Industry" });
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(onSave).not.toHaveBeenCalled();
  });

  it("asks before Escape drops a typed value, and saves nothing", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn(async () => undefined);
    await user.click(renderEdited({ onSave }));
    const box = await screen.findByRole("textbox", { name: "Industry" });
    await user.type(box, "!");
    await user.keyboard("{Escape}");
    await user.click(
      await screen.findByRole("button", { name: en["unsaved.discard"] }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(onSave).not.toHaveBeenCalled();
  });

  it("closes on Cancel without saving", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn(async () => undefined);
    await user.click(renderEdited({ onSave }));
    await user.type(
      await screen.findByRole("textbox", { name: "Industry" }),
      "!",
    );
    await user.click(screen.getByRole("button", { name: en["create.cancel"] }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(onSave).not.toHaveBeenCalled();
  });

  it("sends nothing for a value settled back to the stored one", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn(async () => undefined);
    await user.click(renderEdited({ onSave, settle: (draft) => draft.trim() }));
    await user.type(
      await screen.findByRole("textbox", { name: "Industry" }),
      "  {Enter}",
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(onSave).not.toHaveBeenCalled();
  });

  it("reports a draft as unsaved while it differs from the stored value", async () => {
    const user = userEvent.setup();
    const onDirtyChange = vi.fn();
    await user.click(renderEdited({ onDirtyChange }));
    const box = await screen.findByRole("textbox", { name: "Industry" });
    await user.type(box, "!");
    expect(onDirtyChange).toHaveBeenLastCalledWith(true);
    await user.type(box, "{Backspace}");
    expect(onDirtyChange).toHaveBeenLastCalledWith(false);
  });

  it("starts each opening from the stored value", async () => {
    const user = userEvent.setup();
    const trigger = renderEdited();
    await user.click(trigger);
    await user.type(
      await screen.findByRole("textbox", { name: "Industry" }),
      "!",
    );
    await user.click(screen.getByRole("button", { name: en["create.cancel"] }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await user.click(trigger);
    expect(
      await screen.findByRole("textbox", { name: "Industry" }),
    ).toHaveValue("Software");
  });

  it("sends one write for a double Enter and holds every way out while it is out", async () => {
    const user = userEvent.setup();
    let release = () => {};
    const onSave = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          release = resolve;
        }),
    );
    await user.click(renderEdited({ onSave }));
    const box = await screen.findByRole("textbox", { name: "Industry" });
    await user.clear(box);
    await user.type(box, "Retail{Enter}{Enter}");
    expect(onSave).toHaveBeenCalledTimes(1);
    expect(
      screen.getByRole("button", { name: en["create.cancel"] }),
    ).toBeDisabled();
    expect(
      screen.getByRole("button", { name: en["common.close"] }),
    ).toBeDisabled();
    await user.keyboard("{Escape}");
    expect(screen.getByRole("dialog", { name: "Industry" })).toBeVisible();
    release();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("refuses a typed day it cannot read and keeps the dialog open", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn(async () => undefined);
    await user.click(
      renderEdited({ onSave, control: "date", stored: "2026-12-15" }),
    );
    const box = await screen.findByRole("textbox", { name: "Industry" });
    await user.clear(box);
    await user.type(box, "31/02/2026");
    await user.click(screen.getByRole("button", { name: en["record.save"] }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Enter a date as DD/MM/YYYY.",
    );
    expect(box).toHaveValue("31/02/2026");
    expect(screen.getByRole("dialog", { name: "Industry" })).toBeVisible();
    expect(onSave).not.toHaveBeenCalled();
  });

  it("closes only the calendar on an Escape pressed inside it", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn(async () => undefined);
    await user.click(
      renderEdited({ onSave, control: "date", stored: "2026-12-15" }),
    );
    await user.click(
      await screen.findByRole("button", { name: /^Choose date/ }),
    );
    await screen.findByRole("dialog", { name: /^Choose date/ });
    await user.keyboard("{ArrowRight}{Escape}");
    expect(screen.queryByRole("dialog", { name: /^Choose date/ })).toBeNull();
    expect(screen.getByRole("dialog", { name: "Industry" })).toBeVisible();
    expect(onSave).not.toHaveBeenCalled();
  });

  it("does not save a paragraph on the Ctrl+Enter that picks an input method candidate", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn(async () => undefined);
    await user.click(renderEdited({ onSave, control: "textarea" }));
    const box = await screen.findByRole("textbox", { name: "Industry" });
    await user.type(box, " B2B");
    fireEvent.keyDown(box, { key: "Enter", ctrlKey: true, isComposing: true });
    fireEvent.keyDown(box, { key: "Enter", ctrlKey: true, keyCode: 229 });
    expect(onSave).not.toHaveBeenCalled();
    fireEvent.keyDown(box, { key: "Enter", ctrlKey: true });
    await waitFor(() => expect(onSave).toHaveBeenCalledWith("Software B2B"));
  });
});
