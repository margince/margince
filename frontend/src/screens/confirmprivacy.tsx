// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Card } from "../design-system/atoms";
import { formatDate } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";

type PrivacyInformationPage = components["schemas"]["PrivacyInformationPage"];

// How the installation says a contact was obtained, in the closed vocabulary
// contact_acquisition_evidence uses. Mapped to a sentence rather than rendered
// raw: `purchased_or_imported` is a database value, and whoever reads this
// page did not ask to learn our column names.
//
// A kind with no entry falls back to the honest general sentence rather than
// showing the token — a vocabulary that grows should not leak its spelling onto
// a page a stranger reads.
const SOURCE_LABEL: Record<string, MessageKey> = {
  subject_initiated: "privacynotice.source.subjectInitiated",
  customer_contract: "privacynotice.source.customerContract",
  requested_quote_or_meeting: "privacynotice.source.requested",
  in_person_permission: "privacynotice.source.inPerson",
  referral: "privacynotice.source.referral",
  event_or_form: "privacynotice.source.eventOrForm",
  public_or_business_source: "privacynotice.source.publicSource",
  purchased_or_imported: "privacynotice.source.purchasedOrImported",
  unknown_legacy: "privacynotice.source.unknown",
};

// The rights a subject holds, each named in the reader's own language.
const RIGHT_LABEL: Record<string, MessageKey> = {
  access: "privacynotice.right.access",
  rectification: "privacynotice.right.rectification",
  erasure: "privacynotice.right.erasure",
  restriction: "privacynotice.right.restriction",
  objection: "privacynotice.right.objection",
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
  const tz = viewerZone();

  const sourceKey =
    SOURCE_LABEL[card.acquired_as] ?? "privacynotice.source.unknown";
  const purposes = card.purposes ?? [];

  return (
    <div className="pref-page">
      <h1 className="t-h2">{t("privacynotice.title")}</h1>
      <p className="t-body confirm-intro">{t("privacynotice.intro")}</p>

      <Card>
        <h2 className="t-h3">{t("privacynotice.source.title")}</h2>
        <p className="t-body">{t(sourceKey)}</p>
        {card.acquired_at ? (
          <p className="t-caption">
            {t("privacynotice.source.when", {
              date: formatDate(card.acquired_at, locale, tz),
            })}
          </p>
        ) : null}
      </Card>

      {purposes.length > 0 ? (
        <Card>
          <h2 className="t-h3">{t("privacynotice.purposes.title")}</h2>
          <ul className="confirm-fields">
            {purposes.map((purpose) => (
              <li key={purpose} className="t-body">
                {purpose}
              </li>
            ))}
          </ul>
        </Card>
      ) : null}

      <Card>
        <h2 className="t-h3">{t("privacynotice.rights.title")}</h2>
        <ul className="confirm-fields">
          {card.rights.map((right) => (
            <li key={right} className="t-body">
              {t(RIGHT_LABEL[right] ?? "privacynotice.right.access")}
            </li>
          ))}
        </ul>
        <p className="t-caption">{t("privacynotice.rights.how")}</p>
      </Card>
    </div>
  );
}
