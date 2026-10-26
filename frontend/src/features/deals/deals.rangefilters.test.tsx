/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { ListSurface } from "../../design-system/listsurface";
import { type Locale, LocaleProvider, useLocale, useT } from "../../i18n";
import { dealRangeChips } from "./deals.rangefilters";

// The deal list's commercial dials as a reader meets them in the filter bar.
// Each is picked or typed, applied as one row, and named again after a reload.

afterEach(cleanup);

/** The filter bar with the commercial dials, holding its own state. */
function Bar({
  initial = {},
  baseCurrency = "EUR",
  seen,
}: Readonly<{
  initial?: Record<string, string>;
  baseCurrency?: string;
  seen?: (filters: Record<string, string>) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [chosen, setChosen] = useState(initial);
  return (
    <ListSurface
      chips={dealRangeChips({
        words: { t, locale, zone: "Europe/Berlin" },
        filters: chosen,
        baseCurrency,
        leading: ["EUR", "USD"],
      })}
      chosen={chosen}
      onChipChange={(key, value) => {
        const next = { ...chosen, [key]: value };
        setChosen(next);
        seen?.(next);
      }}
    >
      <p>rows</p>
    </ListSurface>
  );
}

function renderBar(props: Parameters<typeof Bar>[0], locale: Locale = "en") {
  return render(
    <LocaleProvider initial={locale}>
      <Bar {...props} />
    </LocaleProvider>,
  );
}

async function openAttribute(name: string) {
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Filter" }));
  await user.click(screen.getByRole("button", { name }));
  return user;
}

describe("the deal list's commercial filters", () => {
  it("narrows to this fiscal quarter as a word the view keeps", async () => {
    let filters: Record<string, string> = {};
    renderBar({ seen: (next) => (filters = next) });
    const user = await openAttribute("Close date");
    await user.click(
      screen.getByRole("radio", { name: "This fiscal quarter" }),
    );
    expect(filters).toEqual({ close_day: "this_quarter" });
    expect(
      screen.getByRole("group", { name: "Close date: This fiscal quarter" }),
    ).toBeInTheDocument();
  });

  it("applies a typed value range in the base currency's minor units", async () => {
    let filters: Record<string, string> = {};
    renderBar({ seen: (next) => (filters = next) });
    const user = await openAttribute("Value");
    await user.type(
      screen.getByRole("spinbutton", { name: "At least (EUR)" }),
      "20000",
    );
    await user.click(screen.getByRole("button", { name: "Apply" }));
    expect(filters).toEqual({ value: "2000000.." });
    expect(
      screen.getByRole("group", { name: /^Value: From €20,000\.00$/ }),
    ).toBeInTheDocument();
  });

  it("refuses to apply a range with neither end", async () => {
    renderBar({});
    await openAttribute("Value");
    expect(screen.getByRole("button", { name: "Apply" })).toBeDisabled();
    expect(
      screen.getByText("Enter a start, an end or both."),
    ).toBeInTheDocument();
  });

  it("keeps the menu open while a day is picked in the calendar", async () => {
    let filters: Record<string, string> = {};
    renderBar({ seen: (next) => (filters = next) });
    const user = await openAttribute("Close date");
    await user.click(screen.getByRole("button", { name: "Choose date: From" }));
    const calendar = await screen.findByRole("dialog", {
      name: "Choose date: From",
    });
    await user.click(within(calendar).getByRole("button", { name: "Today" }));
    await user.click(screen.getByRole("button", { name: "Apply" }));
    expect(filters.close_day).toMatch(/^\d{4}-\d{2}-\d{2}\.\.$/);
  });

  it("names a range that arrived in the address", () => {
    renderBar({
      initial: { close_day: "2026-10-01..2026-12-31", value: "..5000000" },
    });
    expect(
      screen.getByRole("group", {
        name: "Close date: 01/10/2026 – 31/12/2026",
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("group", { name: /^Value: Up to €50,000\.00$/ }),
    ).toBeInTheDocument();
  });

  it("offers countries by name in the reader's language", async () => {
    let filters: Record<string, string> = {};
    renderBar({ seen: (next) => (filters = next) }, "de");
    const user = await openAttribute("Land");
    await user.click(screen.getByRole("radio", { name: "Deutschland" }));
    expect(filters).toEqual({ country: "DE" });
  });

  it("offers no value dial before the base currency is known", async () => {
    renderBar({ baseCurrency: "" });
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Filter" }));
    expect(screen.queryByRole("button", { name: "Value" })).toBeNull();
    expect(screen.getByRole("button", { name: "Currency" })).toBeTruthy();
  });
});
