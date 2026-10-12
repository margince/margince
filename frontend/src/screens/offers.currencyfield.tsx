import type { components } from "../api/schema";
import { Field } from "../design-system/atoms";
import { Select } from "../design-system/select";
import { useT } from "../i18n";

type Offer = components["schemas"]["Offer"];

/**
 * The currency control for a draft offer, offered only while there is no price
 * to re-mean.
 *
 * A line's price is an integer carrying no unit of its own, so moving the
 * offer's currency would leave every one of them where it is and read it in the
 * new one. The server refuses that, and a control that invites the change only
 * to have it turned down makes the reader discover the rule by being told no —
 * so the rule is the field's hint and the control is disabled instead.
 *
 * Its own file because `offers.tsx` is on a shrink-only length ratchet, and this
 * is the piece of it that stands alone.
 */
export function OfferCurrencyField({
  value,
  lineItems,
  onChange,
}: Readonly<{
  value: string;
  lineItems: Offer["line_items"];
  onChange: (currency: string) => void;
}>) {
  const t = useT();
  // Counted from the lines themselves rather than from the offer's totals, and
  // only where a line states a NON-ZERO price. A line's price is never absent —
  // the column is NOT NULL — so stating 0 is the only way it says "no price",
  // and zero is the same amount in every currency either way. The server counts
  // exactly these lines (`refuseRepricingByCurrency`), and it has to: a control
  // that offers a change the server refuses, or withholds one the server would
  // take, is wrong in whichever direction the two disagree.
  const priced = (lineItems ?? []).filter(
    (line) => line.unit_price_minor !== 0,
  ).length;

  return (
    <Field
      label={t("offer.currency")}
      hint={priced > 0 ? t("offer.currencyFixedByLines") : undefined}
    >
      {(control) => (
        <Select
          {...control}
          disabled={priced > 0}
          value={value}
          onChange={onChange}
          // A currency code is its own label — an ISO 4217 code is not copy, so
          // there is nothing to translate.
          options={["EUR", "USD", "GBP", "CHF"].map((code) => ({
            value: code,
            label: code,
          }))}
        />
      )}
    </Field>
  );
}
