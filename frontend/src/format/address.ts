import { foldForSearch } from "./collate";
import { INTL_LOCALE } from "./intllocale";

// The stored parts of an address, as the API sends them on a record.
export type PostalAddress = Readonly<{
  line1?: string | null;
  line2?: string | null;
  postal_code?: string | null;
  city?: string | null;
  region?: string | null;
  country?: string | null;
}>;

/**
 * The address the way a reader writes it on an envelope, one part per line.
 * Street lines come first, then the postal code before the city, then region
 * and country. The lines are joined on a newline the value column keeps
 * (`white-space: pre-line`, fieldgrid.css). Empty when no part is set.
 */
export function postalLines(address: PostalAddress | null | undefined): string {
  const part = (value: string | null | undefined) => value?.trim() ?? "";
  const city = part(address?.city);
  const place = [part(address?.postal_code), city].filter(Boolean).join(" ");
  const area = [
    part(address?.region),
    countryBeyondCity(city, address?.country),
  ]
    .filter(Boolean)
    .join(", ");
  return [part(address?.line1), part(address?.line2), place, area]
    .filter(Boolean)
    .join("\n");
}

/**
 * The country of an address, or undefined when it only names the city again.
 *
 * A city-state's city and country are one place ("Monaco" in "MC"), and an
 * address that prints both says it twice. The country may be stored as a code
 * or as a name. So the city is held against the stored value and against the
 * code's name in every language this build speaks.
 */
export function countryBeyondCity(
  city: string | null | undefined,
  country: string | null | undefined,
): string | undefined {
  const stored = country?.trim();
  const place = city?.trim();
  if (!stored || !place) {
    return stored || undefined;
  }
  const folded = foldForSearch(place);
  const same = countryNames(stored).some(
    (name) => foldForSearch(name) === folded,
  );
  return same ? undefined : stored;
}

function countryNames(country: string): string[] {
  if (!/^[A-Za-z]{2}$/.test(country)) {
    return [country];
  }
  const code = country.toUpperCase();
  const names = Object.values(INTL_LOCALE).map((tag) =>
    new Intl.DisplayNames(tag, { type: "region" }).of(code),
  );
  return [country, ...names.filter((name): name is string => Boolean(name))];
}
