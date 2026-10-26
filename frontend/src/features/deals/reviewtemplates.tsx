// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { Badge, Button, EmptyState } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryStates } from "../../design-system/querystates";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import {
  type ReviewTemplate,
  useReviewTemplates,
} from "./outcomereview.queries";
import { ReviewTemplateEditor } from "./reviewtemplateeditor";
import "./reviewtemplates.css";

/**
 * The questions a closed deal is asked, one panel per template.
 *
 * The questions are shown in full rather than counted. An administrator opening
 * this page is asking what their reps are being asked, and "3 questions" does
 * not answer that.
 */
export function ReviewTemplatesCard() {
  const t = useT();
  const query = useReviewTemplates();
  const templates = query.data ?? [];
  const canEdit = useCanWrite("custom_field", "update");
  const [editing, setEditing] = useState<ReviewTemplate | null>(null);
  if (!query.isSuccess || templates.length === 0) {
    return (
      <Panel>
        <PanelBody>
          <QueryStates query={query} pendingLabel={t("reviewTemplates.title")}>
            <EmptyState>{t("reviewTemplates.empty")}</EmptyState>
          </QueryStates>
        </PanelBody>
      </Panel>
    );
  }
  return (
    <>
      {templates.map((template) => (
        <TemplatePanel
          key={template.id}
          template={template}
          onEdit={canEdit ? () => setEditing(template) : undefined}
        />
      ))}
      {editing && (
        <ReviewTemplateEditor
          template={editing}
          onClose={() => setEditing(null)}
        />
      )}
    </>
  );
}

function TemplatePanel({
  template,
  onEdit,
}: Readonly<{ template: ReviewTemplate; onEdit?: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const headId = useId();
  return (
    <Panel
      title={<span id={headId}>{template.label}</span>}
      titleAction={
        <span className="reviewtemplate-head-end">
          {/* Retired templates stay listed. An old review names the template it
              came from, so a reader who finds that name here learns it is no
              longer offered rather than meeting a name the product denies. */}
          {!template.active && <Badge>{t("reviewTemplates.retired")}</Badge>}
          {onEdit && (
            <Button aria-describedby={headId} onClick={onEdit}>
              {t("reviewTemplates.edit")}
            </Button>
          )}
        </span>
      }
    >
      <ol className="reviewtemplate-questions" aria-labelledby={headId}>
        {template.questions.map((question, index) => (
          <li
            key={question.key}
            className="panel-row panel-row-record reviewtemplate-question"
          >
            <span
              className="reviewtemplate-number t-caption"
              aria-hidden="true"
            >
              {formatNumber(index + 1, locale)}
            </span>
            <span className="reviewtemplate-label">{question.label}</span>
            {question.required && (
              <Badge>{t("reviewTemplates.required")}</Badge>
            )}
          </li>
        ))}
      </ol>
    </Panel>
  );
}
