import { useState } from "react";
import { useCanWrite } from "../app/capability";
import { Badge, Button } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { useT } from "../i18n";
import {
  type ReviewTemplate,
  useReviewTemplates,
} from "./outcomereview.queries";
import { ReviewTemplateEditor } from "./reviewtemplateeditor";
import "./deal360/deal360.css";

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
  return (
    <Panel title={t("reviewTemplates.title")}>
      <PanelBody>
        {isPending || isError || templates.length === 0 ? (
          <SurfaceState
            state={isPending ? "loading" : isError ? "failed" : "empty"}
            emptyLabel={t("reviewTemplates.empty")}
            loadingLabel={t("reviewTemplates.title")}
          >
            {null}
          </SurfaceState>
        ) : (
          <>
            {templates.map((template) => (
              <div key={template.id}>
                <TemplateRow template={template} />
                {canEdit && (
                  <Button variant="ghost" onClick={() => setEditing(template)}>
                    {t("reviewTemplates.edit")}
                  </Button>
                )}
              </div>
            ))}
            <p>{t("reviewTemplates.editHint")}</p>
            {editing && (
              <ReviewTemplateEditor
                template={editing}
                onClose={() => setEditing(null)}
              />
            )}
          </>
        )}
      </PanelBody>
    </Panel>
  );
}

function TemplateRow({ template }: Readonly<{ template: ReviewTemplate }>) {
  const t = useT();
  return (
    <div className="review-template">
      <div className="review-template-head">
        <span className="t-label">{template.label}</span>
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
      </div>
      <ol className="review-template-questions">
        {template.questions.map((question) => (
          <li key={question.key}>
            {question.label}
            {question.required && (
              <span className="muted"> {t("reviewTemplates.required")}</span>
            )}
          </li>
        ))}
      </ol>
    </div>
  );
}
