/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@testing-library/jest-dom/vitest";
import { useCallback, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { InlineText } from "./inlinetext";

afterEach(cleanup);

function renderCloseDate() {
  const onSave = vi.fn(async (_next: string) => {});
  render(
    <LocaleProvider initial="en">
      <InlineText
        label="Expected close"
        value="2026-03-01"
        display="1 Mar 2026"
        type="date"
        placeholder="Add a date"
        canEdit
        onSave={onSave}
      />
    </LocaleProvider>,
  );
  return { onSave };
}

const trigger = () =>
  screen.getByRole("button", { name: "Change Expected close" });
const dayBox = () =>
  screen.getByRole<HTMLInputElement>("textbox", { name: "Expected close" });

describe("editing a day where it is read", () => {
  it("saves a day picked in the calendar, as Enter does, and puts the reader back on the value", async () => {
    const user = userEvent.setup();
    const { onSave } = renderCloseDate();
    await user.click(trigger());
    expect(dayBox().value).toBe("01/03/2026");

    await user.click(screen.getByRole("button", { name: /^Choose date/ }));
    await screen.findByRole("dialog", { name: /^Choose date/ });
    expect(onSave).not.toHaveBeenCalled();
    await user.click(
      screen.getByRole("button", { name: "Tuesday, 10 March 2026" }),
    );

    await waitFor(() => expect(onSave).toHaveBeenCalledWith("2026-03-10"));
    await waitFor(() => expect(trigger()).toHaveFocus());
    expect(onSave).toHaveBeenCalledTimes(1);
  });

  it("keeps the edit open while the calendar is up, and Escape there closes only the calendar", async () => {
    const user = userEvent.setup();
    const { onSave } = renderCloseDate();
    await user.click(trigger());
    await user.click(screen.getByRole("button", { name: /^Choose date/ }));
    await screen.findByRole("dialog", { name: /^Choose date/ });
    await user.keyboard("{ArrowRight}");

    await user.keyboard("{Escape}");

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(dayBox().value).toBe("01/03/2026");
    expect(onSave).not.toHaveBeenCalled();
  });

  it("closes without a write once focus leaves the field and its calendar", async () => {
    const user = userEvent.setup();
    const { onSave } = renderCloseDate();
    await user.click(trigger());
    await user.click(screen.getByRole("button", { name: /^Choose date/ }));
    await screen.findByRole("dialog", { name: /^Choose date/ });
    await user.keyboard("{Escape}");

    await user.click(document.body);

    expect(
      screen.queryByRole("textbox", { name: "Expected close" }),
    ).toBeNull();
    expect(trigger()).toHaveTextContent("1 Mar 2026");
    expect(onSave).not.toHaveBeenCalled();
  });

  it("saves a day typed in the reader's pattern on Enter", async () => {
    const user = userEvent.setup();
    const { onSave } = renderCloseDate();
    await user.click(trigger());
    await user.clear(dayBox());
    await user.type(dayBox(), "15/04/2026{Enter}");

    await waitFor(() => expect(onSave).toHaveBeenCalledWith("2026-04-15"));
    expect(onSave).toHaveBeenCalledTimes(1);
  });

  it("refuses a typed day it cannot read, and writes nothing over the stored one", async () => {
    const user = userEvent.setup();
    const { onSave } = renderCloseDate();
    await user.click(trigger());
    await user.clear(dayBox());
    await user.type(dayBox(), "31/02/2026{Enter}");

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Enter a date as DD/MM/YYYY.",
    );
    expect(dayBox().value).toBe("31/02/2026");
    expect(onSave).not.toHaveBeenCalled();
  });

  it("keeps the edit open when a press in the calendar focuses nothing", async () => {
    const user = userEvent.setup();
    const { onSave } = renderCloseDate();
    await user.click(trigger());
    await user.click(screen.getByRole("button", { name: /^Choose date/ }));
    await screen.findByRole("grid", { name: "March 2026" });

    await user.click(screen.getByText("March 2026"));

    expect(dayBox()).toBeInTheDocument();
    expect(screen.getByRole("dialog", { name: /^Choose date/ })).toBeVisible();
    expect(onSave).not.toHaveBeenCalled();
  });

  it("saves a day whose press does not move focus to it", async () => {
    const user = userEvent.setup();
    const { onSave } = renderCloseDate();
    await user.click(trigger());
    await user.click(screen.getByRole("button", { name: /^Choose date/ }));
    const day = await screen.findByRole("button", {
      name: "Tuesday, 10 March 2026",
    });

    fireEvent.pointerDown(day);
    fireEvent.mouseDown(day);
    act(() => {
      if (document.activeElement instanceof HTMLElement) {
        document.activeElement.blur();
      }
    });
    fireEvent.pointerUp(day);
    fireEvent.mouseUp(day);
    fireEvent.click(day);

    await waitFor(() => expect(onSave).toHaveBeenCalledWith("2026-03-10"));
    expect(onSave).toHaveBeenCalledTimes(1);
  });

  it("does not save on the Enter that confirms an input method candidate", async () => {
    const user = userEvent.setup();
    const { onSave } = renderCloseDate();
    await user.click(trigger());
    await user.clear(dayBox());
    await user.type(dayBox(), "15/04/2026");

    fireEvent.keyDown(dayBox(), { key: "Enter", isComposing: true });

    expect(onSave).not.toHaveBeenCalled();
    expect(dayBox().value).toBe("15/04/2026");

    fireEvent.keyDown(dayBox(), { key: "Enter" });

    await waitFor(() => expect(onSave).toHaveBeenCalledWith("2026-04-15"));
    expect(onSave).toHaveBeenCalledTimes(1);
  });

  it("saves a typed day when the reader presses outside the field", async () => {
    const user = userEvent.setup();
    const { onSave } = renderCloseDate();
    await user.click(trigger());
    await user.clear(dayBox());
    await user.type(dayBox(), "15/04/2026");

    await user.click(document.body);

    await waitFor(() => expect(onSave).toHaveBeenCalledWith("2026-04-15"));
    expect(onSave).toHaveBeenCalledTimes(1);
  });

  it("keeps an unreadable typed day open with its one refusal when the reader leaves", async () => {
    const user = userEvent.setup();
    const { onSave } = renderCloseDate();
    await user.click(trigger());
    await user.clear(dayBox());
    await user.type(dayBox(), "31/02/2026");

    await user.click(document.body);

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Enter a date as DD/MM/YYYY.",
    );
    expect(dayBox().value).toBe("31/02/2026");
    expect(onSave).not.toHaveBeenCalled();
  });

  it("closes on Enter over the stored day without a write", async () => {
    const user = userEvent.setup();
    const { onSave } = renderCloseDate();
    await user.click(trigger());

    await user.type(dayBox(), "{Enter}");

    await waitFor(() => expect(trigger()).toHaveFocus());
    expect(onSave).not.toHaveBeenCalled();
  });

  it("saves a typed day when the reader tabs away", async () => {
    const user = userEvent.setup();
    const { onSave } = renderCloseDate();
    await user.click(trigger());
    await user.clear(dayBox());
    await user.type(dayBox(), "15/04/2026");
    await user.tab();
    await user.tab();

    await waitFor(() => expect(onSave).toHaveBeenCalledWith("2026-04-15"));
  });

  it("moves to the calendar button in one Tab when the parent re-renders on the draft", async () => {
    const user = userEvent.setup();
    render(
      <LocaleProvider initial="en">
        <DirtyHeardParent />
      </LocaleProvider>,
    );
    await user.click(trigger());
    await user.clear(dayBox());
    await user.type(dayBox(), "22/12/2026");

    await user.tab();

    expect(screen.getByRole("button", { name: /^Choose date/ })).toHaveFocus();
    expect(dayBox().value).toBe("22/12/2026");
  });
});

// Hands InlineText a new onEditingChange each time the draft turns dirty.
function DirtyHeardParent() {
  const [dirty, setDirty] = useState(false);
  const onEditingChange = useCallback(
    (editing: boolean) => {
      if (!editing && dirty) setDirty(false);
    },
    [dirty],
  );
  return (
    <InlineText
      label="Expected close"
      value="2026-03-01"
      type="date"
      placeholder="Add a date"
      canEdit
      onEditingChange={onEditingChange}
      onDirtyChange={setDirty}
      onSave={async () => {}}
    />
  );
}
