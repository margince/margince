import { Badge } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { useT } from "../i18n";
import {
  type ReviewTemplate,
  useReviewTemplates,
} from "./outcomereview.queries";
import "./deal360/deal360.css";

/**
 * The questions a closed deal is asked, per outcome.
 *
 * Read-only, and the card says so rather than showing controls that refuse. The
 * API serves these templates and does not yet accept edits, so a form here
 * would be a promise the server breaks — see the note the card renders.
 *
 * The questions are shown in full rather than counted. An administrator opening
 * this page is asking what their reps are being asked, and "3 questions" does
 * not answer that.
 */
export function ReviewTemplatesCard() {
  const t = useT();
  const { data, isPending, isError } = useReviewTemplates();
  const templates = data ?? [];
  return (
    <Panel title={t("reviewTemplates.title")} sub={t("reviewTemplates.sub")}>
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
              <TemplateRow key={template.id} template={template} />
            ))}
            <p className="t-caption">{t("reviewTemplates.readOnly")}</p>
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
        <Badge quiet tone={template.outcome === "won" ? "success" : "danger"}>
          {t(
            template.outcome === "won"
              ? "outcomeReview.outcomeWon"
              : "outcomeReview.outcomeLost",
          )}
        </Badge>
        {/* Retired templates stay listed. An old review names the template it
            came from, so a reader who finds that name here learns it is no
            longer offered rather than meeting a name the product denies. */}
        {!template.active && (
          <Badge quiet>{t("reviewTemplates.retired")}</Badge>
        )}
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
