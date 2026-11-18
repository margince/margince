// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Globe } from "lucide-react";
import { useState } from "react";
import { Button } from "../../design-system/atoms";
import { Field } from "../../design-system/field";
import { Popover } from "../../design-system/popover";
import { Select } from "../../design-system/select";
import { countryOptions } from "../../format/assignedcountries";
import { jurisdictionName } from "../../format/jurisdiction";
import { type Translator, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  type RecipientCountries,
  type RecipientCountryReading,
  statedFor,
  useRecordCountry,
} from "./composecountries";

const SOURCE_KEY: Partial<
  Record<NonNullable<RecipientCountryReading["source"]>, MessageKey>
> = {
  contact_address: "compose.country.fromContact",
  employer_address: "compose.country.fromEmployer",
  lead_company_address: "compose.country.fromLeadCompany",
};

/**
 * The country one recipient's marketing rules follow, on their chip: what the
 * record says and where it says it, and the sender's own setting for this
 * message. Nothing here writes to the contact.
 */
export function RecipientCountryControl({
  address,
  countries,
  disabled,
}: Readonly<{
  address: string;
  countries: RecipientCountries;
  disabled?: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [open, setOpen] = useState(false);
  const { reading, settled } = useRecordCountry(address, open);
  const stated = statedFor(countries.stated, address);
  const recorded = reading?.country ?? null;
  const shown = stated ? stated.country : recorded;
  const options = countryOptions(locale).map((option) => ({
    value: option.value.toLowerCase(),
    label: option.label,
  }));
  return (
    <Popover
      className="compose-country-trigger"
      dialog
      disabled={disabled}
      open={open}
      onOpenChange={setOpen}
      label={
        <>
          <Globe size={12} aria-hidden="true" />
          {shown && (
            <span aria-hidden="true">{jurisdictionName(shown, locale)}</span>
          )}
          <span className="sr-only">
            {t("compose.country.open", { address })}
          </span>
        </>
      }
    >
      <div className="compose-country-panel">
        <p role="status">{sourceLine(t, stated, reading, settled, locale)}</p>
        <Field label={t("compose.country.label")}>
          {(control) => (
            <Select
              {...control}
              value={shown ?? ""}
              placeholder={t("compose.country.none")}
              options={options}
              onChange={(code) => countries.state(address, code)}
            />
          )}
        </Field>
        <div className="compose-country-actions">
          {stated?.country !== null && (
            <Button
              variant="ghost"
              onClick={() => countries.state(address, null)}
            >
              {t("compose.country.clear")}
            </Button>
          )}
          {stated && (
            <Button variant="ghost" onClick={() => countries.forget(address)}>
              {t("compose.country.useRecord")}
            </Button>
          )}
        </div>
      </div>
    </Popover>
  );
}

function sourceLine(
  t: Translator,
  stated: ReturnType<typeof statedFor>,
  reading: RecipientCountryReading | undefined,
  settled: boolean,
  locale: ReturnType<typeof useLocale>["locale"],
): string {
  if (stated) {
    return stated.country === null
      ? t("compose.country.cleared")
      : t("compose.country.yours", {
          country: jurisdictionName(stated.country, locale),
        });
  }
  if (!settled) {
    return t("compose.country.reading");
  }
  if (!reading?.country) {
    return t("compose.country.unknown");
  }
  const country = jurisdictionName(reading.country, locale);
  // A source a newer server names that this build does not know still says
  // the country.
  const key = reading.source ? SOURCE_KEY[reading.source] : undefined;
  return key ? t(key, { country }) : country;
}
