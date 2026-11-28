/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RecordFormBody } from "../../components/create.form";
import { LocaleProvider, translate } from "../../i18n";
import { en as englishCopy } from "../../i18n/en";
import { dealEditFields } from "./deals.fields";

const t = (
  key: Parameters<typeof translate>[1],
  params?: Record<string, string>,
) => translate("en", key, params);

function moneyFields(currency: string, keys: readonly string[]) {
  return dealEditFields(t, {
    partnerOptions: [],
    masked: [],
    closed: false,
    me: "u1",
    currentOwner: null,
    currency,
    leadingCurrencies: ["EUR"],
    locale: "en",
  }).filter((field) => keys.includes(field.key));
}

afterEach(cleanup);

function Form({
  currency,
  keys,
  seeded,
  save,
}: Readonly<{
  currency: string;
  keys: readonly string[];
  seeded: Record<string, string>;
  save: (values: Record<string, string>) => void;
}>) {
  const [values, setValues] = useState(seeded);
  return (
    <LocaleProvider initial="en">
      <RecordFormBody
        fields={moneyFields(currency, keys)}
        values={values}
        setValues={setValues}
        rows={{}}
        setRows={() => {}}
        pending={false}
        error={null}
        intent="save"
        onClose={() => {}}
        onSubmit={save}
      />
    </LocaleProvider>
  );
}

const MONEY = ["amount", "expected_arr", "currency"];
const PRICED = { amount: "5000", expected_arr: "1200", currency: "EUR" };

// Converted, an amount the currency cannot hold is NaN, which a request sends
// as null: a cleared Expected ARR, or a refusal that names columns.
describe("a deal amount the currency cannot hold", () => {
  it.each([
    [
      "Expected ARR",
      "1200.999",
      "This amount has more decimals than EUR allows.",
    ],
    ["Value", "1.999", "This amount has more decimals than EUR allows."],
    ["Value", "-3", "Enter an amount of zero or more."],
    [
      "Value",
      "900000000000000",
      "Enter an amount of at most 90071992547409.91.",
    ],
  ])(
    "refuses %s %s under the field and sends nothing",
    (label, typed, sentence) => {
      const save = vi.fn();
      render(<Form currency="EUR" keys={MONEY} seeded={PRICED} save={save} />);
      const box = screen.getByRole("textbox", { name: label });
      fireEvent.change(box, { target: { value: typed } });
      expect(screen.getByText(sentence)).toBeInTheDocument();
      fireEvent.click(screen.getByRole("button", { name: "Save" }));
      expect(save).not.toHaveBeenCalled();
      expect(box).toHaveFocus();
    },
  );

  it("checks against the currency on the form, not the one the deal had", () => {
    render(
      <Form
        currency="EUR"
        keys={MONEY}
        seeded={{ ...PRICED, amount: "1000.5", currency: "JPY" }}
        save={vi.fn()}
      />,
    );
    expect(
      screen.getByText("This amount has more decimals than JPY allows."),
    ).toBeInTheDocument();
  });

  // An offer sets the currency, so the amount's dialog shows the amount alone.
  it("checks an amount edited alone against the deal's currency", () => {
    render(
      <Form
        currency="JPY"
        keys={["amount"]}
        seeded={{ amount: "1000.5" }}
        save={vi.fn()}
      />,
    );
    expect(
      screen.getByText("This amount has more decimals than JPY allows."),
    ).toBeInTheDocument();
  });

  it("lets an amount the currency holds through", () => {
    const save = vi.fn();
    render(<Form currency="EUR" keys={MONEY} seeded={PRICED} save={save} />);
    fireEvent.change(screen.getByRole("textbox", { name: "Value" }), {
      target: { value: "1.99" },
    });
    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(save.mock.calls[0]?.[0]).toMatchObject({ amount: "1.99" });
  });

  it("refuses a blank Amount on a won deal that has a price, and allows it on an open one", () => {
    const amountOf = (closed: boolean) =>
      dealEditFields(t, {
        partnerOptions: [],
        masked: [],
        closed,
        priced: true,
        me: "u1",
        currentOwner: null,
        currency: "EUR",
        leadingCurrencies: ["EUR"],
        locale: "en",
      }).find((field) => field.key === "amount");
    expect(amountOf(true)?.validate?.("", {})).toBe(
      englishCopy["deal.closedAmountKept"],
    );
    expect(amountOf(false)?.validate?.("", {})).toBeUndefined();
  });
});
