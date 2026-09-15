// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Sparkles } from "lucide-react";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { forReader } from "../format/collate";
import { formatDayMonth, formatNumber, INTL_LOCALE } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { contactTabRoute } from "./contacttab";

type Contact360 = components["schemas"]["Contact360"];

/**
 * The one ask this overview makes of a reader about the enrichment pass
 * itself, rather than about the contact: a handful of fields a machine read
 * and nobody has ruled on yet.
 *
 * A field carrying a verdict already (corrected, confirmed, or the reader
 * decided not to be asked again) is a field somebody has ruled on, so it is
 * not this callout's business; `contact.enriched.*` on Data & tools is where
 * every field lives, judged one at a time. This card exists because a rep
 * opening the overview before a call should not have to go looking for the
 * six-row audit to learn that six things are still unconfirmed; it says so
 * here, and sends them to the row where they act on it.
 *
 * Silent once nothing is pending: a reader who has confirmed or corrected
 * everything the pass has read owes this card nothing more.
 */
export function ContactConfirmCallout({
  view,
}: Readonly<{ view: Contact360 }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const pending = (view.profile_fields ?? []).filter(
    (field) => field.verdict === undefined,
  );
  if (pending.length === 0) {
    return null;
  }
  // Distinct field names, alphabetised in the reader's own order rather than
  // the order the pass happened to read them in: a list a reader compares
  // against the fields on the page above should not reorder itself depending
  // on which email arrived first.
  const fieldNames = [
    ...new Set(
      pending.map((field) => t(`contact.enriched.field.${field.field}`)),
    ),
  ].sort((a, b) => forReader(a, b, locale));
  const fields = new Intl.ListFormat(INTL_LOCALE[locale], {
    style: "long",
    type: "conjunction",
  }).format(fieldNames);
  // The most recent read among the fields this callout is naming, not the
  // most recent read overall: a field already ruled on says nothing about
  // when THESE were read.
  const latest = pending.reduce(
    (max, field) => (field.captured_at > max ? field.captured_at : max),
    pending[0].captured_at,
  );
  return (
    <Callout
      tone="ai"
      kind="standing"
      icon={Sparkles}
      title={plural("contact.confirm.title", pending.length, {
        count: formatNumber(pending.length, locale),
      })}
      actions={
        <Button
          variant="link"
          onClick={() => navigate(contactTabRoute(view.contact.id, "research"))}
        >
          {t("contact.confirm.review")}
        </Button>
      }
    >
      {t("contact.confirm.body", {
        fields,
        when: formatDayMonth(latest, locale, zone),
      })}
    </Callout>
  );
}
