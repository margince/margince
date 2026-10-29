import { useEffect } from "react";
import { isLocale, type Locale, useLocale } from "../../i18n";
import { parseParams } from "../../lib/urlstate";

/**
 * Take the language from the link a reader followed.
 *
 * A message goes out in one language and its footer links carry that
 * language, so the page they open should speak it too — a German mail
 * whose unsubscribe page answers in English has changed language halfway
 * through one conversation.
 *
 * It ADOPTS rather than sets: `?lang=` is what the message happened to be
 * written in, not a choice this reader made, so it must not be written
 * back as their stored pick. Someone who then uses the switcher IS
 * choosing, and that path persists.
 *
 * An absent or unrecognised value does nothing at all, leaving the stored
 * pick and then the browser to decide, which is what a public page falls
 * back to anyway.
 */
export function usePublicLocale(): void {
  const { adoptLocale } = useLocale();
  useEffect(() => {
    const asked = parseParams(globalThis.location.hash).get("lang");
    if (asked !== undefined && isLocale(asked)) {
      adoptLocale(asked);
    }
  }, [adoptLocale]);
}

/**
 * Take the language the server says the mail behind this page was sent in.
 * Adopted like `?lang=`, so it never becomes the reader's stored pick.
 */
export function useMailLanguage(language: Locale | undefined): void {
  const { adoptLocale } = useLocale();
  useEffect(() => {
    if (language) {
      adoptLocale(language);
    }
  }, [language, adoptLocale]);
}
