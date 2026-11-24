// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import type { components } from "../../api/schema";
import {
  SourceReviewDialog,
  useCanChangeNoticeCase,
} from "../../components/sourcereviewdialog";
import { Button } from "../../design-system/atoms";
import { useToast } from "../../design-system/toast";
import { acquisitionKindLabel } from "../../format/acquisitionkind";
import { formatDate } from "../../format/format";
import { type Translator, useLocale, useT } from "../../i18n";
import { actorAttribution, qualifierPhrase } from "../../lib/auditactor";
import { useViewerId } from "../../lib/session";
import {
  SOURCE_ANSWER_LABEL,
  SOURCE_ANSWERS,
  sourceKindFor,
} from "../../lib/sourcequestion";
import { useViewerZone } from "../../lib/viewerzone";

type ConfirmedSource = components["schemas"]["ContactConfirmedSource"];
type Attribution = components["schemas"]["PrincipalAttribution"];

/**
 * The contact's source in its details: the one a reviewer confirmed, and any
 * source still waiting for confirmation. A reader who may assess it answers
 * the question in a drawer over the record. Any other reader is told who
 * confirms it, and gets no button that would lead nowhere.
 */
export function ContactSourceReview({
  reviews,
  confirmed,
}: Readonly<{ reviews: readonly string[]; confirmed?: ConfirmedSource }>) {
  return (
    <>
      {confirmed && <ConfirmedSourceLines confirmed={confirmed} />}
      <OpenSourceReview reviews={reviews} />
    </>
  );
}

function OpenSourceReview({
  reviews,
}: Readonly<{ reviews: readonly string[] }>) {
  const t = useT();
  const toast = useToast();
  const canWork = useCanChangeNoticeCase();
  const [open, setOpen] = useState(false);
  const first = reviews[0];
  if (!first) return null;
  return (
    <>
      <p>{t("notice.source.needsConfirmation")}</p>
      {reviews.length > 1 && (
        <p className="t-caption">{t("notice.source.more")}</p>
      )}
      {canWork ? (
        <Button variant="link" onClick={() => setOpen(true)}>
          {t("notice.source.review")}
        </Button>
      ) : (
        <p className="t-caption">{t("notice.source.contactExplanation")}</p>
      )}
      {open && (
        <SourceReviewDialog
          key={first}
          caseId={first}
          open
          onClose={() => setOpen(false)}
          onConfirmed={() => toast.show(t("notice.source.saved"))}
        />
      )}
    </>
  );
}

function ConfirmedSourceLines({
  confirmed,
}: Readonly<{ confirmed: ConfirmedSource }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const viewerId = useViewerId();
  const source = confirmedSourceLabel(confirmed.source_kind, t);
  const confirmedOn = formatDate(confirmed.confirmed_at, locale, zone);
  return (
    <>
      <p>
        {confirmed.source_obtained_at
          ? t("notice.source.confirmedSource", {
              source,
              date: formatDate(confirmed.source_obtained_at, locale, zone),
            })
          : source}
      </p>
      <p className="t-caption">
        {confirmedByLine(confirmed.confirmed_by, viewerId, confirmedOn, t)}
      </p>
    </>
  );
}

// The kind in the words the review question offered for it ("Met face to
// face"), else the acquisition vocabulary's own label.
function confirmedSourceLabel(kind: string, t: Translator): string {
  const answer = SOURCE_ANSWERS.find(
    (candidate) => sourceKindFor(candidate, "") === kind,
  );
  return answer
    ? t(SOURCE_ANSWER_LABEL[answer])
    : acquisitionKindLabel(kind, t);
}

function confirmedByLine(
  by: Attribution | undefined,
  viewerId: string | undefined,
  date: string,
  t: Translator,
): string {
  if (!by) return t("notice.source.confirmedAt", { date });
  const who = actorAttribution(by, viewerId);
  if (who.labelKey === "audit.you") {
    return t("notice.source.confirmedByYou", { date });
  }
  const qualifier = qualifierPhrase(who, t);
  const actor = [
    who.name ?? (who.labelKey ? t(who.labelKey) : who.identifier),
    qualifier,
  ]
    .filter(Boolean)
    .join(" ");
  return t("notice.source.confirmedBy", { actor, date });
}
