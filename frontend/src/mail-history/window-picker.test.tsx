/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
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
