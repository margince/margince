// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { categoryNamesTogether } from "../../lib/provider-categories";
import {
  canEnrichNow,
  useProviderConnections,
} from "../../lib/provider-status";
import { useContact360 } from "./contact360";
import type { Contact, Profile } from "./contactprovider";
import type { useEnrichRun } from "./contactprovider.run";

/** The priced details, each behind its own button with its own price.
 *
 *  Automatic enrichment takes only what the provider gives away, so these are
 *  the ones nobody has bought yet and nobody will until a reader decides this
 *  particular contact is worth it. The price is on the button because the
 *  decision is what to spend, and a button that hid its cost would be asking
 *  somebody to agree to a number they cannot see.
 */
export function BuyPriced({
  contactId,
  profile,
  enrich,
  running,
}: Readonly<{
  contactId: string;
  profile: Profile;
  enrich: ReturnType<typeof useEnrichRun>;
  // Whether a run is still moving, from the panel that already asked. Not
  // recomputed here: two answers to "is a lookup happening" would drift, and
  // the one that drifts wrong leaves a spending button on screen.
  running: boolean;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const connections = useProviderConnections();
  const connection = connections.data?.connections.find(
    (c) => c.provider === profile.provider,
  );
  // What the record holds, which differs from what this provider returned. An
  // address typed by hand or captured from a mailbox is billed for again, like
  // one Surfe sold us. Reading `profile.emails` alone leaves that charge unsaid.
  //
  // Read here rather than handed down from the mount sites: there are two of
  // them today and a third would have to remember. This is the query the page
  // around it already made, under the same key, so it costs a cache read.
  const record = useContact360(contactId);
  const held = heldCategories(profile, record.data?.contact);
  // Only what this connection carries, asked of every category the press
  // would send, including those beyond the one on the button.
  //
  // A category with a prerequisite sends both. Take a connection with the
  // mobile on and the work email off: it would show a two-credit button whose
  // request the server always refuses. Asking about the press's own set keeps
  // the server's refusal and this button in agreement.
  const enabled = (category: string) =>
    connection?.configuration.categories?.[category] ?? false;
  // Offered on the button's own category alone. The detail sought decides
  // whether there is anything to buy. A mobile is still worth offering when its
  // work email is held; that is the common case, and the note below prices it.
  // A press whose own category is held would only re-buy what is on screen.
  const buyable = (connection?.catalog ?? []).filter(
    (entry) =>
      !entry.free &&
      boughtWith(entry).every(enabled) &&
      !held.has(entry.category),
  );
  // Without the record there is nothing to check a re-buy against. A spending
  // button that cannot say what it pays for twice is the defect this row
  // prevents. The page that mounts this has already asked, so the wait is short.
  if (
    !record.data ||
    buyable.length === 0 ||
    !canEnrichNow(profile.state, running)
  ) {
    return null;
  }
  return (
    <div className="pe-buy-row">
      {buyable.map((entry) => {
        const asks = boughtWith(entry);
        const rebought = asks.filter((category) => held.has(category));
        return (
          <div className="pe-buy-offer" key={entry.category}>
            <Button
              type="button"
              pending={enrich.isPending}
              busyLabel={t("provider.profile.lookingUp")}
              onClick={() =>
                enrich.mutate({
                  contactId,
                  provider: profile.provider,
                  categories: asks,
                })
              }
            >
              {plural("provider.profile.buy", creditsOf(entry), {
                // The category being SOUGHT, not everything the press sends.
                // Naming a detail already on the record would read as an
                // offer to buy what the reader can see they have.
                category: categoryNamesTogether(
                  asks.filter((category) => !held.has(category)),
                  t,
                  locale,
                ),
                credits: formatNumber(creditsOf(entry), locale),
              })}
            </Button>
            {rebought.length > 0 && (
              // The price above is the whole press, and part of it buys again
              // what this record holds. Surfe charges one credit per pool that
              // returns anything, and does not know we hold the address. It
              // refuses to look for a number with the email flag off. So the
              // second charge is the vendor's rule, and it is said plainly here.
              <p>
                {t("provider.profile.buyRebuys", {
                  categories: categoryNamesTogether(rebought, t, locale),
                })}
              </p>
            )}
          </div>
        );
      })}
    </div>
  );
}

/** What one press actually asks for: the category, and the one it can only be
 *  bought alongside.
 *
 *  Surfe's mobile lookup is the case. Asked for on its own, the provider hunts
 *  for an email nobody bought, fails, and skips the number — returning a run
 *  that COMPLETED with nothing in it, which the page then reports as a
 *  successful lookup over an empty field. The server refuses the lone request
 *  now; this is what stops the button making it.
 *
 *  `entry.cost` is already the price of the pair, so the figure beside this
 *  needs no adjusting: the server prices a category together with the one it
 *  requires (`pricedWith`, backend/internal/modules/integrations/store.go),
 *  so cost and request are derived from one descriptor rule. Held by
 *  TestAPricedCategoryCarriesItsPrerequisitesPrice
 *  (backend/internal/modules/integrations/catalog_test.go), which fails if a
 *  catalog entry ever quotes a lone price for a paired purchase.
 *
 *  The button names both, joined by the locale's conjunction rather than a
 *  comma — a comma reads as a list to pick from, and a rep who read one as the
 *  work-email button bought a mobile number he had not asked for. */
function boughtWith(
  entry: components["schemas"]["ProviderCategoryCost"],
): string[] {
  // The mirror of `pricedWith` (backend/internal/modules/integrations/store.go):
  // the category, the trigger whose empty answer fires it as a fallback, and the
  // prerequisite it needs an answer from. The server refuses a request missing
  // either. `cost` prices the whole set, so a press naming less buys half a pair.
  //
  // Both are walked, beyond the first that answers. Nothing refuses a
  // descriptor declaring a category with each. The price would then name three
  // categories while the press named two.
  const alongside = [entry.follows, entry.requires].filter(
    (category): category is string => Boolean(category),
  );
  return [...new Set([...alongside, entry.category])];
}

/** The credits one category costs, summed across pools. A category priced in
 *  two pools is one purchase, and two figures on one button would read as a
 *  choice between them. */
function creditsOf(
  entry: components["schemas"]["ProviderCategoryCost"],
): number {
  return Object.values(entry.cost).reduce((total, n) => total + n, 0);
}

/** The purchase categories this record already shows, whatever put them there.
 *
 *  Consulted TWICE about one press, for two different questions. Whether to
 *  offer the button reads the category being sought — a mobile is worth
 *  offering when the work email it needs is already held. What the button says
 *  reads every category the press sends, because the provider bills for
 *  whatever it returns and cannot be told to skip an address we hold, so that
 *  press pays for the email a second time.
 *
 *  Answering only the first question is what let the second charge go
 *  unmentioned; asking only the provider's own answer is what let it go
 *  unmentioned for an address the provider never returned.
 */
function heldCategories(
  profile: Profile,
  contact: Contact | undefined,
): ReadonlySet<string> {
  const held = new Set<string>();
  for (const email of profile.emails) {
    const bought = PROVIDER_EMAIL_CATEGORY[email.email_type ?? ""];
    if (bought) {
      held.add(bought);
    }
  }
  for (const email of contact?.emails ?? []) {
    const filed = RECORD_EMAIL_CATEGORY[email.email_type];
    if (filed) {
      held.add(filed);
    }
  }
  if (
    profile.mobile_phones.length > 0 ||
    (contact?.phones ?? []).some((phone) => phone.phone_type === "mobile")
  ) {
    held.add("mobile");
  }
  return held;
}

// Keyed by address type. The two are separate purchases from separate pools.
// Collapsing them hides each behind the other: a contact with a personal
// address would never be offered a work email, and the reverse.
//
// Both vocabularies stop short of the whole enum. An address the provider
// could not classify counts for neither, nor does one the record files as
// `other`. Guessing which type it is could hide a usable offer or claim a
// re-buy that does not happen. Offering a purchase they may decline is better.
const PROVIDER_EMAIL_CATEGORY: Readonly<Record<string, string>> = {
  professional: "professional_email",
  personal: "personal_email",
};

const RECORD_EMAIL_CATEGORY: Readonly<Record<string, string>> = {
  work: "professional_email",
  personal: "personal_email",
};
