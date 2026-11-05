/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ReactNode, useState } from "react";
import { createPortal } from "react-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import {
  openView,
  openViewStep,
  viewButton,
} from "../testing/listview.testkit";
import { type ListChip, ListSurface } from "./listsurface";

// The View panel from the keyboard, as a phone's modal sheet, and under a
// dialog one of its steps raises.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function render(ui: ReactNode) {
  return rtlRender(<LocaleProvider initial="en">{ui}</LocaleProvider>);
}

function stubWidth(width: number) {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: width <= Number(/max-width:\s*(\d+)px/.exec(query)?.[1] ?? 0),
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
}

const STATUS: ListChip = {
  key: "status",
  label: "Status",
  allLabel: "Any status",
  options: [{ value: "open", label: "Open" }],
};

/** A chip whose value step raises a dialog of its own, as a date field does. */
const CLOSE_DATE: ListChip = {
  key: "close",
  label: "Close date",
  allLabel: "Any date",
  options: [],
  appliedName: undefined,
  editor: () => <RaisedDialog />,
};

function RaisedDialog() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        Pick a day
      </button>
      {open &&
        createPortal(
          <div role="dialog" aria-modal="true" aria-label="Calendar">
            <button type="button">5 November</button>
          </div>,
          document.body,
        )}
    </>
  );
}

function Surface({
  chips = [STATUS],
}: Readonly<{ chips?: readonly ListChip[] }>) {
  const [sort, setSort] = useState("");
  const [chosen, setChosen] = useState<Record<string, string>>({});
  return (
    <ListSurface
      search={{ value: "", onChange: () => undefined }}
      sort={{ value: sort, onChange: setSort }}
      sortOptions={[{ field: "name", label: "Name" }]}
      chips={chips}
      chosen={chosen}
      onChipChange={(key, value) =>
        setChosen((prev) => ({ ...prev, [key]: value }))
      }
      archived={{ checked: false, onChange: () => undefined }}
    >
      <p>rows</p>
    </ListSurface>
  );
}

describe("the View panel from the keyboard", () => {
  it("lands on the first row, then walks the rows and Show archived", async () => {
    const user = userEvent.setup();
    render(<Surface />);
    (await viewButton()).focus();
    await user.keyboard("{Enter}");
    const panel = await screen.findByRole("dialog");

    const order = [document.activeElement?.getAttribute("aria-label")];
    await user.tab();
    order.push(document.activeElement?.getAttribute("aria-label"));
    await user.tab();
    order.push(document.activeElement?.textContent);
    expect(order).toEqual([
      "Filters, None",
      "Sort, Default order",
      "Show archived",
    ]);
    expect(panel.contains(document.activeElement)).toBe(true);
  });
});

describe("the View panel on a phone", () => {
  it("is a modal sheet headed View, and a press on its heading keeps it open", async () => {
    stubWidth(390);
    const user = userEvent.setup();
    render(<Surface />);

    const sheet = await openView(user);
    expect(sheet).toHaveAttribute("aria-modal", "true");
    await user.click(within(sheet).getByRole("heading", { name: "View" }));

    expect(screen.getByRole("dialog")).toBe(sheet);
  });

  // Were the surface's Escape to run beside the sheet's, it would hand focus to
  // a control inside the leaving sheet.
  it("closes once on Escape from a step and hands focus to View", async () => {
    stubWidth(390);
    const user = userEvent.setup();
    render(<Surface />);

    await openViewStep(user, "Sort");
    await user.keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    const view = await viewButton();
    expect(view).toHaveFocus();
    expect(view).toHaveAttribute("aria-expanded", "false");

    await user.keyboard("{Escape}");
    expect(view).toHaveFocus();
  });
});

describe("a dialog a step raises", () => {
  it("owns its own presses, and the panel stays open under it", async () => {
    stubWidth(1440);
    const user = userEvent.setup();
    render(<Surface chips={[STATUS, CLOSE_DATE]} />);

    const step = await openViewStep(user, "Filters");
    await user.click(within(step).getByRole("button", { name: "Close date" }));
    await user.click(screen.getByRole("button", { name: "Pick a day" }));
    await user.click(screen.getByRole("button", { name: "5 November" }));

    expect(await viewButton()).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("group", { name: "Close date" })).toBeTruthy();
  });
});
