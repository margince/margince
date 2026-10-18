/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { LocaleProvider } from "../../i18n";
import {
  EMPTY_LINE_BILLING,
  type LineBilling,
  OfferLineBillingFields,
} from "./offerlinebilling";

afterEach(cleanup);

// The add-line form holds the billing state; this stands in for it so the
// three controls run as they do there.
function Harness({ seen }: Readonly<{ seen: LineBilling[] }>) {
  const [value, setValue] = useState(EMPTY_LINE_BILLING);
  return (
    <LocaleProvider initial="en">
      <OfferLineBillingFields
        value={value}
        onChange={(next) => {
          seen.push(next);
          setValue(next);
        }}
      />
    </LocaleProvider>
  );
}

type User = ReturnType<typeof userEvent.setup>;

async function pick(user: User, field: string, choice: string) {
  await pickOption(
    user,
    await screen.findByRole("combobox", { name: field }),
    choice,
  );
}

function periodSelect() {
  return screen.queryByRole("combobox", { name: "Billing period" });
}

describe("OfferLineBillingFields", () => {
  it("offers a period only while the line is recurring, and drops it on leaving", async () => {
    const seen: LineBilling[] = [];
    const user = userEvent.setup();
    render(<Harness seen={seen} />);
    expect(periodSelect()).toBeNull();
    await pick(user, "Billing", "One-time");
    expect(periodSelect()).toBeNull();
    await pick(user, "Billing", "Recurring");
    await pick(user, "Billing period", "Quarterly");
    expect(seen.at(-1)?.billingIntervalMonths).toBe("3");
    await pick(user, "Billing", "One-time");
    expect(periodSelect()).toBeNull();
    expect(seen.at(-1)).toEqual({
      billingModel: "one_time",
      billingIntervalMonths: "",
      intervalCount: "",
    });
    await pick(user, "Billing", "Recurring");
    expect(periodSelect()).not.toBeNull();
  });
});
