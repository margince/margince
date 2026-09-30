/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { ImportWindowPicker } from "./window-picker";

afterEach(cleanup);
it("shows only the selected window's server date, and tolerates older servers", () => {
  const onChange = vi.fn();
  const preview = {
    window: "120m",
    after_date: "2016-09-14",
    computed_at: "2026-09-14T12:00:00Z",
    estimated_messages: 20,
  } satisfies NonNullable<Parameters<typeof ImportWindowPicker>[0]["preview"]>;
  const { rerender } = render(
    <LocaleProvider initial="en">
      <ImportWindowPicker value="120m" onChange={onChange} preview={preview} />
    </LocaleProvider>,
  );
  expect(screen.getByText(/Imports email since/)).toHaveTextContent("2016");
  rerender(
    <LocaleProvider initial="en">
      <ImportWindowPicker value="84m" onChange={onChange} preview={preview} />
    </LocaleProvider>,
  );
  expect(screen.queryByText(/Imports email since/)).toBeNull();
  rerender(
    <LocaleProvider initial="en">
      <ImportWindowPicker
        value="120m"
        onChange={onChange}
        preview={{
          window: "120m",
          computed_at: preview.computed_at,
          estimated_messages: 20,
        }}
      />
    </LocaleProvider>,
  );
  expect(screen.queryByText(/Imports email since/)).toBeNull();
  expect(
    screen.getByRole("combobox", { name: "Import window" }),
  ).toHaveTextContent("10 years");
  expect(onChange).not.toHaveBeenCalled();
});

/** The Select renders its options only once opened, so this helper takes the
 *  same two steps somebody using the screen takes. */
async function openedWindowLabels(): Promise<(string | null)[]> {
  await userEvent.click(screen.getByRole("combobox"));
  return within(screen.getByRole("listbox"))
    .getAllByRole("option")
    .map((option) => option.textContent);
}

it("offers only the windows this installation admits", async () => {
  render(
    <LocaleProvider initial="en">
      <ImportWindowPicker
        value="12m"
        onChange={vi.fn()}
        offered={["3m", "6m", "12m", "24m"]}
      />
    </LocaleProvider>,
  );
  const options = await openedWindowLabels();
  expect(options).toHaveLength(4);
  // The ten-year window is what a cap is usually set to exclude, and offering
  // it would offer a choice the server refuses at both doors.
  expect(options.join("|")).not.toMatch(/10 years|120/);
});

it("falls back to the whole set when the server names none", async () => {
  const { unmount } = render(
    <LocaleProvider initial="en">
      <ImportWindowPicker value="12m" onChange={vi.fn()} />
    </LocaleProvider>,
  );
  const all = (await openedWindowLabels()).length;
  expect(all).toBeGreaterThan(4);
  unmount();

  // An EMPTY list is the same fallback, not an empty picker: a missing field
  // must not leave somebody unable to import at all.
  render(
    <LocaleProvider initial="en">
      <ImportWindowPicker value="12m" onChange={vi.fn()} offered={[]} />
    </LocaleProvider>,
  );
  expect(await openedWindowLabels()).toHaveLength(all);
});
