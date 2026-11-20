// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { Badge, Button, EmptyState } from "../../design-system/atoms";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { PanelNotices } from "../../design-system/panelnotices";
import { QueryStates } from "../../design-system/querystates";
import { Switch } from "../../design-system/switch";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { shippedLabel } from "../../lib/shippedlabels";
import {
  type ReviewTemplate,
  useReviewTemplates,
  useSetReviewTemplateActive,
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
  const canCreate = useCanWrite("custom_field", "create");
  const canEdit = useCanWrite("custom_field", "update");
  const setActive = useSetReviewTemplateActive();
  const refusalId = useId();
  // null is closed; "new" opens the drawer that adds a template.
  const [editing, setEditing] = useState<ReviewTemplate | "new" | null>(null);
  return (
    <>
      <Panel
        title={t("reviewTemplates.title")}
        titleAction={
          canCreate && (
            <Button onClick={() => setEditing("new")}>
              {t("reviewTemplates.add")}
            </Button>
          )
        }
      >
        <PanelBody>
          <PanelIntro>{t("reviewTemplates.intro")}</PanelIntro>
          {(!query.isSuccess || templates.length === 0) && (
            <QueryStates
              query={query}
              pendingLabel={t("reviewTemplates.title")}
            >
              <EmptyState>{t("reviewTemplates.empty")}</EmptyState>
            </QueryStates>
          )}
        </PanelBody>
        <PanelNotices
          refused={
            setActive.isError
              ? {
                  title: t("leadSources.notSaved"),
                  error: setActive.error,
                  id: refusalId,
                }
              : undefined
          }
        />
      </Panel>
      {templates.map((template) => (
        <TemplatePanel
          key={template.id}
          template={template}
          canEdit={canEdit}
          refusedBy={
            setActive.isError && setActive.variables?.id === template.id
              ? refusalId
              : undefined
          }
          onActive={(active) =>
            setActive.mutate({
              id: template.id,
              version: template.version,
              active,
            })
          }
          onEdit={canEdit ? () => setEditing(template) : undefined}
        />
      ))}
      {editing && (
        <ReviewTemplateEditor
          template={editing === "new" ? undefined : editing}
          onClose={() => setEditing(null)}
        />
      )}
    </>
  );
}

function TemplatePanel({
  template,
  canEdit,
  refusedBy,
  onActive,
  onEdit,
}: Readonly<{
  template: ReviewTemplate;
  canEdit: boolean;
  refusedBy?: string;
  onActive: (active: boolean) => void;
  onEdit?: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const headId = useId();
  const name = shippedLabel(template, t);
  return (
    <Panel
      title={<span id={headId}>{name}</span>}
      titleAction={
        <span className="reviewtemplate-head-end">
          {/* Retired templates stay listed. An old review names the template it
              came from, so a reader who finds that name here learns it is no
              longer offered rather than meeting a name the product denies. */}
          {!template.active && <Badge>{t("reviewTemplates.retired")}</Badge>}
          <Switch
            label={t("reviewTemplates.activeFor", { label: name })}
            labelHidden
            describedBy={refusedBy}
            checked={template.active}
            disabled={!canEdit}
            onChange={onActive}
          />
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
            <span className="reviewtemplate-label">
              {shippedLabel(question, t)}
            </span>
            {question.required && (
              <Badge>{t("reviewTemplates.required")}</Badge>
            )}
          </li>
        ))}
      </ol>
    </Panel>
  );
}
