// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { useCanWrite } from "../app/capability";
import { Badge, Button } from "../design-system/atoms";
import {
  Panel,
  PanelBody,
  PanelGroupHead,
  PanelIntro,
} from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import {
  type ReviewTemplate,
  useReviewTemplates,
} from "./outcomereview.queries";
import { ReviewTemplateEditor } from "./reviewtemplateeditor";
import "./reviewtemplates.css";

/**
 * The questions a closed deal is asked, per outcome.
 *
 * Administrators edit future questions; saved reviews keep their snapshots.
 *
 * The questions are shown in full rather than counted. An administrator opening
 * this page is asking what their reps are being asked, and "3 questions" does
 * not answer that.
 */
export function ReviewTemplatesCard() {
  const t = useT();
  const { data, isPending, isError } = useReviewTemplates();
  const templates = data ?? [];
  const canEdit = useCanWrite("custom_field", "update");
  const [editing, setEditing] = useState<ReviewTemplate | null>(null);
  if (isPending || isError || templates.length === 0) {
    return (
      <Panel title={t("reviewTemplates.title")}>
        <PanelBody>
          <SurfaceState
            state={isPending ? "loading" : isError ? "failed" : "empty"}
            emptyLabel={t("reviewTemplates.empty")}
            loadingLabel={t("reviewTemplates.title")}
          >
            {null}
          </SurfaceState>
        </PanelBody>
      </Panel>
    );
  }
  return (
    <Panel title={t("reviewTemplates.title")}>
      <PanelBody>
        <PanelIntro>{t("reviewTemplates.editHint")}</PanelIntro>
      </PanelBody>
      {templates.map((template) => (
        <TemplateGroup
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
    </Panel>
  );
}

function TemplateGroup({
  template,
  onEdit,
}: Readonly<{ template: ReviewTemplate; onEdit?: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const headId = useId();
  return (
    <>
      <PanelGroupHead
        title={template.label}
        level="h3"
        id={headId}
        action={
          <span className="reviewtemplate-head-end">
            <Badge tone={template.outcome === "won" ? "success" : "danger"}>
              {t(
                template.outcome === "won"
                  ? "outcomeReview.outcomeWon"
                  : "outcomeReview.outcomeLost",
              )}
            </Badge>
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
      />
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
    </>
  );
}
