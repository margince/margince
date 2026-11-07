// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { EntityRef } from "../../components/entityref";
import {
  SourceReviewDialog,
  useCanChangeNoticeCase,
} from "../../components/sourcereviewdialog";
import { Button } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";
import type { NoticeCase } from "./noticecases.logic";

export function NoticeSourceReview({ duty }: Readonly<{ duty: NoticeCase }>) {
  const t = useT();
  const canWork = useCanChangeNoticeCase();
  const [open, setOpen] = useState(false);
  return (
    <Panel title={t("notice.source.title")}>
      <PanelBody>
        <p>{t("notice.source.explanation")}</p>
        {duty.contact_name && (
          <EntityRef
            kind="contact"
            id={duty.contact_id}
            name={duty.contact_name}
          />
        )}
        {canWork && (
          <Button onClick={() => setOpen(true)}>
            {t("notice.source.review")}
          </Button>
        )}
        {open && (
          <SourceReviewDialog
            key={duty.id}
            caseId={duty.id}
            open={open}
            onClose={() => setOpen(false)}
          />
        )}
      </PanelBody>
    </Panel>
  );
}
