/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { RecordFormBody } from "../../components/create.form";
import { LocaleProvider, translate } from "../../i18n";
import { dealEditFields } from "./deals.fields";
import { mapDealUpdate } from "./deals.write";

const fields = dealEditFields((key) => translate("en", key), {
  partnerOptions: [],
  masked: [],
  closed: false,
  me: "u1",
  currentOwner: null,
  currency: "",
  leadingCurrencies: ["EUR"],
  locale: "en",
}).filter((field) =>
  ["name", "amount", "expected_arr", "currency"].includes(field.key),
);
const seeded = { name: "Before", amount: "", expected_arr: "", currency: "" };
afterEach(cleanup);

function Form({ save }: { save: (body: unknown) => void }) {
  const [values, setValues] = useState<Record<string, string>>(seeded);
  return (
    <LocaleProvider initial="en">
      <RecordFormBody
        fields={fields}
        values={values}
        setValues={setValues}
        rows={{}}
        setRows={() => {}}
        pending={false}
        error={null}
        intent="save"
        onClose={() => {}}
        onSubmit={(submitted) => save(mapDealUpdate(submitted, seeded))}
      />
    </LocaleProvider>
  );
}

it("renames an unpriced deal without inventing a currency", () => {
  const save = vi.fn();
  render(<Form save={save} />);
  fireEvent.change(screen.getByRole("textbox", { name: "Deal name" }), {
    target: { value: "After" },
  });
  expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  expect(save).toHaveBeenCalledWith({ name: "After" });
});

it.each(["Value", "Expected ARR"])(
  "requires currency for %s, including zero, until the figure is cleared",
  (label) => {
    render(<Form save={vi.fn()} />);
    const input = screen.getByRole("spinbutton", { name: label });
    fireEvent.change(input, { target: { value: "0" } });
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    fireEvent.change(input, { target: { value: "" } });
    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
  },
);
