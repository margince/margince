// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import {
  SourceReviewDialog,
  useCanChangeNoticeCase,
} from "../../components/sourcereviewdialog";
import { Button } from "../../design-system/atoms";
import { useT } from "../../i18n";

/**
 * The contact's unconfirmed source, in its details. A reader who may assess
 * it answers the question here, in a drawer over the record. Any other reader
 * is told who confirms it, and gets no button that would lead nowhere.
 */
export function ContactSourceReview({
  reviews,
}: Readonly<{ reviews: readonly string[] }>) {
  const t = useT();
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
        />
      )}
    </>
  );
}
