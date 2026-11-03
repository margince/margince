import { foldForSearch } from "./collate";
import { INTL_LOCALE } from "./intllocale";

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
