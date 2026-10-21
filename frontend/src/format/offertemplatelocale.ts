import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";

export const OFFER_TEMPLATE_LOCALES: { value: string; label: MessageKey }[] = [
  { value: "de-DE", label: "template.localeDE" },
  { value: "en-US", label: "template.localeEN" },
];

export function templateLanguage(locale: string, t: Translator): string {
  const known = OFFER_TEMPLATE_LOCALES.find(
    (option) => option.value === locale,
  );
  return known ? t(known.label) : locale;
}
