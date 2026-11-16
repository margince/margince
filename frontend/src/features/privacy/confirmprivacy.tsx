// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Card } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { formatDate } from "../../format/format";
import { browserZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  type AcquisitionKind,
  isAcquisitionKind,
} from "../../lib/acquisitionkinds";
import { labelFor } from "../../lib/labelfor";
import { labelOf } from "../../lib/preferences.logic";
import {
  BASIS_LABEL,
  ControllerCard,
  HeldCard,
  ObjectionCard,
  RetentionCard,
} from "./confirmprivacyparts";
import { PublicPage } from "./publicpage";

type PrivacyInformationPage = components["schemas"]["PrivacyInformationPage"];

// How the installation says a contact was obtained, in the closed vocabulary
// contact_acquisition_evidence uses. Mapped to a sentence rather than rendered
// raw: `purchased_or_imported` is a database value, and whoever reads this
// page did not ask to learn our column names.
//
// A kind with no entry falls back to the honest general sentence rather than
// showing the token — a vocabulary that grows should not leak its spelling onto
// a page a stranger reads.
const SOURCE_LABEL: Readonly<Record<AcquisitionKind, MessageKey>> = {
  subject_initiated: "privacynotice.source.subjectInitiated",
  customer_contract: "privacynotice.source.customerContract",
  requested_quote_or_meeting: "privacynotice.source.requested",
  in_person_permission: "privacynotice.source.inPerson",
  referral: "privacynotice.source.referral",
  event_or_form: "privacynotice.source.eventOrForm",
  public_or_business_source: "privacynotice.source.publicSource",
  purchased_or_imported: "privacynotice.source.purchasedOrImported",
  crm_migration: "privacynotice.source.crmMigration",
  mailbox_history: "privacynotice.source.mailboxHistory",
  unknown_legacy: "privacynotice.source.unknown",
};

// Where the user who created a contact by hand said it came from.
const STATED_SOURCE_LABEL: Readonly<
  Record<NonNullable<PrivacyInformationPage["stated_source"]>, MessageKey>
> = {
  met_in_person: "privacynotice.source.stated.metInPerson",
  business_card: "privacynotice.source.stated.businessCard",
  referral: "privacynotice.source.stated.referral",
  website: "privacynotice.source.stated.website",
  other: "privacynotice.source.stated.other",
};

// The rights a subject holds, each named in the reader's own language.
const RIGHT_LABEL: Record<string, MessageKey> = {
  access: "privacynotice.right.access",
  rectification: "privacynotice.right.rectification",
  erasure: "privacynotice.right.erasure",
  restriction: "privacynotice.right.restriction",
  portability: "privacynotice.right.portability",
  objection: "privacynotice.right.objection",
  withdraw_consent: "privacynotice.right.withdrawConsent",
  complain_to_authority: "privacynotice.right.complain",
};

// PrivacyNotice is the page a notice link opens: what is held, where it came
// from, what it is for, and the rights over it.
//
// IT HAS NO FORM, and that absence is the design. The mail that carried this
// link asks nothing and says so; the submit path refuses any answer arriving on
// it. This is also the one link a contact who asked us to stop still receives,
// because the disclosure duty survives their stop — so a page offering them a
// subscription box would be the re-engagement surface that stop exists to
// prevent, reached by a message they cannot refuse.
export function PrivacyNotice({
  card,
}: Readonly<{ card: PrivacyInformationPage }>) {
  const t = useT();
  const { locale } = useLocale();
  const tz = browserZone();

  const sourceKey = isAcquisitionKind(card.acquired_as)
    ? SOURCE_LABEL[card.acquired_as]
    : "privacynotice.source.unknown";
  // A stated source stands in for "unknown"; a known kind keeps its sentence.
  const stated = card.stated_source;
  const showKind = !stated || sourceKey !== "privacynotice.source.unknown";
  const keys = card.purpose_keys ?? [];
  const since = card.purpose_since ?? [];
  const controllerName =
    card.controller?.legal_name ?? t("privacynotice.basisOn.us");
  const purposes = (card.purposes ?? []).map((label, i) => ({
    name: labelOf(t, { key: keys[i] ?? "", label }),
    basis: card.purpose_bases[i],
    since: since[i] ?? null,
  }));
  // The basis as the clause the purpose card reads: a consent with its date,
  // an agreement with the controller named, or that none is recorded. Any
  // other basis keeps its own sentence.
  const basisClause = (purpose: (typeof purposes)[number]): string | null => {
    if (purpose.basis === "consent" && purpose.since) {
      return t("privacynotice.basisOn.consent", {
        date: formatDate(purpose.since, locale, tz),
      });
    }
    if (purpose.basis === "none") {
      return t("privacynotice.basisOn.none");
    }
    if (purpose.basis === "contract") {
      return t("privacynotice.basisOn.agreement", {
        controller: controllerName,
      });
    }
    return null;
  };
  // The right to object has a card of its own, so the list leaves it out.
  const rights = card.rights.filter((right) => right !== "objection");

  const contact = card.controller?.privacy_contact;
  return (
    <PublicPage>
      <Heading size="xlarge" className="t-h2">
        {t("privacynotice.title")}
      </Heading>
      <p className="t-body confirm-intro">{t("privacynotice.intro")}</p>

      <ControllerCard controller={card.controller} />

      <Card>
        <Heading size="large" className="t-h3">
          {t("privacynotice.source.title")}
        </Heading>
        {showKind ? <p className="t-body">{t(sourceKey)}</p> : null}
        {stated ? (
          <p className="t-body">{t(labelFor(STATED_SOURCE_LABEL, stated))}</p>
        ) : null}
        {card.stated_source_note ? (
          <p className="t-body">
            {t("privacynotice.source.statedNote", {
              note: card.stated_source_note,
            })}
          </p>
        ) : null}
        {card.acquired_at ? (
          <p className="t-caption">
            {t("privacynotice.source.when", {
              date: formatDate(card.acquired_at, locale, tz),
            })}
          </p>
        ) : null}
      </Card>

      <Card>
        <Heading size="large" className="t-h3">
          {t("privacynotice.purposes.title")}
        </Heading>
        <ul className="confirm-fields">
          {purposes.map((purpose) => {
            const clause = basisClause(purpose);
            return (
              <li key={purpose.name} className="t-body">
                {clause
                  ? t("privacynotice.purposeLine", {
                      purpose: purpose.name,
                      basis: clause,
                    })
                  : purpose.name}
                {!clause && purpose.basis ? (
                  <span className="t-caption confirm-basis">
                    {t(labelFor(BASIS_LABEL, purpose.basis))}
                  </span>
                ) : null}
              </li>
            );
          })}
        </ul>
      </Card>

      <HeldCard card={card} />
      <RetentionCard retention={card.retention} />

      <Card>
        <Heading size="large" className="t-h3">
          {t("privacynotice.transfers.title")}
        </Heading>
        <p className="t-body">{t("privacynotice.transfers.body")}</p>
      </Card>

      <Card>
        <Heading size="large" className="t-h3">
          {t("privacynotice.rights.title")}
        </Heading>
        <ul className="confirm-fields">
          {/* A language whose law names two rights in one place writes them
              as two lines of one value; each line is its own item. */}
          {rights.flatMap((right) =>
            t(RIGHT_LABEL[right] ?? "privacynotice.right.access")
              .split("\n")
              .map((line) => (
                <li key={`${right}-${line}`} className="t-body">
                  {line}
                </li>
              )),
          )}
        </ul>
        <p>
          {contact
            ? t("privacynotice.rights.howContact", { contact })
            : t("privacynotice.rights.how")}
        </p>
      </Card>

      {card.rights.includes("objection") ? (
        <ObjectionCard route={card.controller?.objection_route} />
      ) : null}
    </PublicPage>
  );
}
