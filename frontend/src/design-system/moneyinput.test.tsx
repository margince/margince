/** @vitest-environment happy-dom */
import {
  cleanup,
  fireEvent,
  render as rtlRender,
  screen,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { MoneyInput } from "./moneyinput";

// MoneyInput wraps the existing TextInput (field.tsx). It shows and edits
// major units, and emits the minor units the API stores. The scale is the one
// the currency carries (format/minorunits), not a hard-coded hundred.
//
// The displayed text is the component's OWN state, resynced from the
// external valueMinor only when it changes for a reason other than this
// input's own typing (see moneyinput.tsx) — so, unlike a naive
// `value={(valueMinor / 100).toFixed(2)}` controlled input, real
// keystroke-by-keystroke typing never gets fought by a reformat mid-edit.
// The sequential-fireEvent tests below replay actual keystrokes rather than
// one finished string, to pin exactly that.

afterEach(cleanup);

describe("MoneyInput", () => {
  it("reads in its display form at rest and as a plain number while edited", () => {
    const onChangeMinor = vi.fn();
    rtlRender(
      <MoneyInput
        currency="EUR"
        valueMinor={1500000}
        onChangeMinor={onChangeMinor}
        display={(minor) => `EUR ${minor / 100}`}
        aria-label="Goal"
      />,
    );
    const input = screen.getByLabelText("Goal") as HTMLInputElement;
    expect(input.value).toBe("EUR 15000");
    fireEvent.focus(input);
    expect(input.value).toBe("15000.00");
    fireEvent.change(input, { target: { value: "20000" } });
    expect(onChangeMinor).toHaveBeenCalledWith(2000000);
    fireEvent.blur(input);
    expect(input.value).toBe("EUR 20000");
  });

  it("keeps the editor while a box that started empty is typed into", () => {
    rtlRender(
      <MoneyInput
        currency="EUR"
        valueMinor={0}
        blankWhenZero
        onChangeMinor={vi.fn()}
        display={(minor) => `EUR ${minor / 100}`}
        aria-label="Goal"
      />,
    );
    const input = screen.getByLabelText("Goal") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "1" } });
    expect(input.value).toBe("1");
    fireEvent.blur(input);
    expect(input.value).toBe("EUR 1");
  });

  it("empties for good when the caller takes a clear, and keeps the amount when it does not", () => {
    const onClear = vi.fn();
    rtlRender(
      <>
        <MoneyInput
          currency="EUR"
          valueMinor={10000}
          onChangeMinor={vi.fn()}
          onClear={onClear}
          aria-label="Clearable"
        />
        <MoneyInput
          currency="EUR"
          valueMinor={10000}
          onChangeMinor={vi.fn()}
          aria-label="Priced"
        />
      </>,
    );
    for (const name of ["Clearable", "Priced"]) {
      const input = screen.getByLabelText(name) as HTMLInputElement;
      fireEvent.change(input, { target: { value: "" } });
      fireEvent.blur(input);
    }
    expect(onClear).toHaveBeenCalledTimes(1);
    expect((screen.getByLabelText("Clearable") as HTMLInputElement).value).toBe(
      "",
    );
    expect((screen.getByLabelText("Priced") as HTMLInputElement).value).toBe(
      "100.00",
    );
  });

  it("displays the initial value in major units to two decimals", () => {
    rtlRender(
      <MoneyInput
        currency="EUR"
        valueMinor={150000}
        onChangeMinor={vi.fn()}
        aria-label="Amount"
      />,
    );
    const input = screen.getByLabelText("Amount") as HTMLInputElement;
    expect(input.value).toBe("1500.00");
  });

  // A number box takes its decimal sign from the browser, so an English
  // browser read "33,33" as 3333. A text box reads both signs alike.
  it.each([
    ["EUR", "33,33", 3333],
    ["EUR", "0,5", 50],
    ["EUR", "1250,5", 125_050],
  ])("%s: %s commits %i minor units", (currency, typed, want) => {
    const onChangeMinor = vi.fn();
    rtlRender(
      <MoneyInput
        currency={currency}
        valueMinor={0}
        onChangeMinor={onChangeMinor}
        aria-label="Amount"
      />,
    );
    const input = screen.getByLabelText("Amount") as HTMLInputElement;
    expect(input.type).toBe("text");
    expect(input.inputMode).toBe("decimal");
    fireEvent.change(input, { target: { value: typed } });
    expect(onChangeMinor).toHaveBeenLastCalledWith(want);
  });

  // The defect: this input multiplied by 100 whatever the currency, so an
  // offer line typed as 18,000,000 dong reached the API as 1,800,000,000 —
  // a hundred times the price, on a document a buyer signs.
  it.each([
    ["VND", "18000000", 18_000_000],
    ["JPY", "950000", 950_000],
    ["KWD", "95", 95_000],
    ["EUR", "95", 9_500],
  ])("%s: typing %s emits %i minor units", (currency, typed, wantMinor) => {
    const onChangeMinor = vi.fn();
    rtlRender(
      <MoneyInput
        currency={currency}
        valueMinor={0}
        onChangeMinor={onChangeMinor}
        aria-label="Amount"
      />,
    );
    fireEvent.change(screen.getByLabelText("Amount"), {
      target: { value: typed },
    });
    expect(onChangeMinor).toHaveBeenLastCalledWith(wantMinor);
  });

  // A figure with more decimals than the currency HAS is held, not rounded.
  // `step` only governs native validity — onChange still fires — so a
  // fractional dong reached toMinorUnits, rounded, and an offer line saved the
  // altered amount on blur.
  it.each([
    ["VND", "1.5"],
    ["VND", "18000000.25"],
    ["EUR", "12.345"],
    ["KWD", "12.3456"],
  ])(
    "%s: %s has more decimals than the currency, so nothing is committed",
    (currency, typed) => {
      const onChangeMinor = vi.fn();
      rtlRender(
        <MoneyInput
          currency={currency}
          valueMinor={0}
          onChangeMinor={onChangeMinor}
          aria-label="Amount"
        />,
      );
      fireEvent.change(screen.getByLabelText("Amount"), {
        target: { value: typed },
      });
      expect(onChangeMinor).not.toHaveBeenCalled();
    },
  );

  // Nobody types a price as an exponent, and reading one is how `15e-1` once
  // slipped a half dong past the precision guard.
  it.each([
    ["VND", "15e-1"],
    ["VND", "1.5e0"],
    ["EUR", "1005e-3"],
    ["VND", "150e-1"],
    ["EUR", "1.2345e2"],
  ])("%s: %s is refused, whatever it would scale to", (currency, typed) => {
    const onChangeMinor = vi.fn();
    rtlRender(
      <MoneyInput
        currency={currency}
        valueMinor={0}
        onChangeMinor={onChangeMinor}
        aria-label="Amount"
      />,
    );
    fireEvent.change(screen.getByLabelText("Amount"), {
      target: { value: typed },
    });
    expect(onChangeMinor).not.toHaveBeenCalled();
  });

  // And the boundary on the other side: exactly as many decimals as the
  // currency carries is a perfectly good amount and must still commit.
  it.each([
    ["EUR", "12.34", 1234],
    ["KWD", "12.345", 12_345],
    ["VND", "18000000", 18_000_000],
  ])("%s: %s is within the currency's precision", (currency, typed, want) => {
    const onChangeMinor = vi.fn();
    rtlRender(
      <MoneyInput
        currency={currency}
        valueMinor={0}
        onChangeMinor={onChangeMinor}
        aria-label="Amount"
      />,
    );
    fireEvent.change(screen.getByLabelText("Amount"), {
      target: { value: typed },
    });
    expect(onChangeMinor).toHaveBeenLastCalledWith(want);
  });

  // The read direction, which is the half that made the write bug survive a
  // round trip looking correct.
  it("seeds a zero-decimal amount without inventing a fractional part", () => {
    rtlRender(
      <MoneyInput
        currency="VND"
        valueMinor={18_000_000}
        onChangeMinor={vi.fn()}
        aria-label="Amount"
      />,
    );
    expect((screen.getByLabelText("Amount") as HTMLInputElement).value).toBe(
      "18000000",
    );
  });

  it("emits minor units for a whole-number major input", () => {
    const onChangeMinor = vi.fn();
    rtlRender(
      <MoneyInput
        currency="EUR"
        valueMinor={0}
        onChangeMinor={onChangeMinor}
        aria-label="Amount"
      />,
    );
    const input = screen.getByLabelText("Amount");
    fireEvent.change(input, { target: { value: "1500" } });
    expect(onChangeMinor).toHaveBeenLastCalledWith(150000);
  });

  it("emits minor units for a fractional major input", () => {
    const onChangeMinor = vi.fn();
    rtlRender(
      <MoneyInput
        currency="EUR"
        valueMinor={0}
        onChangeMinor={onChangeMinor}
        aria-label="Amount"
      />,
    );
    const input = screen.getByLabelText("Amount");
    fireEvent.change(input, { target: { value: "19.99" } });
    expect(onChangeMinor).toHaveBeenLastCalledWith(1999);
  });

  it("forwards standard input props such as disabled", () => {
    rtlRender(
      <MoneyInput
        currency="EUR"
        valueMinor={0}
        onChangeMinor={vi.fn()}
        aria-label="Amount"
        disabled
      />,
    );
    const input = screen.getByLabelText("Amount") as HTMLInputElement;
    expect(input.disabled).toBe(true);
  });

  it("never reformats the buffer mid-edit, so typing digit-by-digit reaches the intended amount", () => {
    // A controlled input whose value is `(valueMinor / 100).toFixed(2)`
    // recomputed every render snaps "1" to "1.00" the instant it commits,
    // so the next keystroke lands on the reformatted string instead of the
    // one the user is building — typing "125" one digit at a time would
    // never reach 125.00. Replaying each keystroke's resulting string here
    // (as a real <input> hands the DOM) proves the buffer isn't fought.
    const onChangeMinor = vi.fn();
    rtlRender(
      <MoneyInput
        currency="EUR"
        valueMinor={0}
        onChangeMinor={onChangeMinor}
        aria-label="Amount"
      />,
    );
    const input = screen.getByLabelText("Amount") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "1" } });
    fireEvent.change(input, { target: { value: "12" } });
    fireEvent.change(input, { target: { value: "125" } });
    expect(input.value).toBe("125");
    expect(onChangeMinor).toHaveBeenLastCalledWith(12500);
  });

  it("does not commit 0 while the field is empty mid-edit", () => {
    const onChangeMinor = vi.fn();
    rtlRender(
      <MoneyInput
        currency="EUR"
        valueMinor={150000}
        onChangeMinor={onChangeMinor}
        aria-label="Amount"
      />,
    );
    const input = screen.getByLabelText("Amount") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "" } });
    expect(input.value).toBe("");
    expect(onChangeMinor).not.toHaveBeenCalled();
  });

  it("snaps the buffer back to the last committed value on blur", () => {
    const onChangeMinor = vi.fn();
    rtlRender(
      <MoneyInput
        currency="EUR"
        valueMinor={150000}
        onChangeMinor={onChangeMinor}
        aria-label="Amount"
      />,
    );
    const input = screen.getByLabelText("Amount") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "19.9" } });
    fireEvent.blur(input);
    expect(input.value).toBe("19.90");
  });

  it("resyncs the buffer when valueMinor changes from outside (a different row swapped in)", () => {
    const onChangeMinor = vi.fn();
    const { rerender } = rtlRender(
      <MoneyInput
        currency="EUR"
        valueMinor={150000}
        onChangeMinor={onChangeMinor}
        aria-label="Amount"
      />,
    );
    rerender(
      <MoneyInput
        currency="EUR"
        valueMinor={500}
        onChangeMinor={onChangeMinor}
        aria-label="Amount"
      />,
    );
    const input = screen.getByLabelText("Amount") as HTMLInputElement;
    expect(input.value).toBe("5.00");
  });
});

// In German "." groups thousands: "1.500" is fifteen hundred to the reader and
// one and a half to the decimal rule, so it is refused and names both forms.
describe("MoneyInput in German", () => {
  function renderGerman(onChangeMinor: (minor: number) => void) {
    rtlRender(
      <LocaleProvider initial="de">
        <MoneyInput
          currency="EUR"
          valueMinor={9900}
          onChangeMinor={onChangeMinor}
          aria-label="Betrag"
        />
      </LocaleProvider>,
    );
    return screen.findByLabelText("Betrag") as Promise<HTMLInputElement>;
  }

  it("opens on the stored amount written with a comma", async () => {
    expect((await renderGerman(vi.fn())).value).toBe("99,00");
  });

  it("commits 12,5 as 1250 cents", async () => {
    const onChangeMinor = vi.fn();
    const input = await renderGerman(onChangeMinor);
    fireEvent.change(input, { target: { value: "12,5" } });
    expect(onChangeMinor).toHaveBeenLastCalledWith(1250);
  });

  it("refuses 1.500 typed key by key, and puts the amount back", async () => {
    const onChangeMinor = vi.fn();
    const input = await renderGerman(onChangeMinor);
    fireEvent.focus(input);
    for (const key of ["1", "1.", "1.5", "1.50", "1.500"]) {
      fireEvent.change(input, { target: { value: key } });
    }
    fireEvent.blur(input);
    expect(onChangeMinor).toHaveBeenLastCalledWith(9900);
    expect(input.value).toBe("1.500");
    expect(input.getAttribute("aria-invalid")).toBe("true");
    const alert = screen.getByRole("alert");
    expect(alert.textContent).toBe("Gib den Betrag als 1500 oder 1500,00 ein.");
    expect(input.getAttribute("aria-describedby")).toBe(alert.id);
    fireEvent.change(input, { target: { value: "1500" } });
    expect(screen.queryByRole("alert")).toBeNull();
    expect(onChangeMinor).toHaveBeenLastCalledWith(150_000);
  });
});
