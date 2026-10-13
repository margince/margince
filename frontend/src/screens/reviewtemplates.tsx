import { Info } from "lucide-react";
import { Fragment, useState } from "react";
import { useCanWrite } from "../app/capability";
import { routeHash } from "../app/router";
import { Badge, Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { CellStack } from "../design-system/cellstack";
import { DataTable } from "../design-system/datatable";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { useT } from "../i18n";
import { sentenceParts } from "./brief.sentence";
import {
  type ReviewQuestion,
  type ReviewTemplate,
  useReviewTemplates,
} from "./outcomereview.queries";
import { ReviewTemplateEditor } from "./reviewtemplateeditor";
import { ReviewTemplateSummary } from "./reviewtemplatesummary";
import { settingsHref } from "./settingsrouting";
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
            emptyLabel={t("reviewTemplates.emptyLine")}
            loadingLabel={t("reviewTemplates.title")}
          >
            {null}
          </SurfaceState>
        </PanelBody>
      </Panel>
    );
  }
  return (
    <>
      <HowReviewsReachReps />
      {templates.map((template) => (
        <TemplatePanel
          key={template.id}
          template={template}
          onEdit={canEdit ? () => setEditing(template) : undefined}
        />
      ))}
      <Callout tone="info" title={t("reviewTemplates.outcomeNote.title")}>
        <OutcomeNoteBody />
      </Callout>
      {editing && (
        <ReviewTemplateEditor
          template={editing}
          onClose={() => setEditing(null)}
        />
      )}
    </>
  );
}

// The note names the two neighbouring pages inside its own sentence, so the
// translation keeps the holes and each hole becomes the link to that page.
const NOTE_LINKS: Readonly<Record<string, SettingsLink>> = {
  pipelines: { page: "pipelines", label: "settings.tab.pipelines" },
  stageAutomation: {
    page: "stageautomation",
    label: "settings.tab.stageautomation",
  },
};

type SettingsLink = Readonly<{
  page: "pipelines" | "stageautomation";
  label: "settings.tab.pipelines" | "settings.tab.stageautomation";
}>;

function OutcomeNoteBody() {
  const t = useT();
  return (
    <p>
      {sentenceParts(t("reviewTemplates.outcomeNote.body")).map((part, at) => {
        const link = part.kind === "slot" ? NOTE_LINKS[part.name] : undefined;
        if (part.kind === "text" || !link) {
          return (
            // biome-ignore lint/suspicious/noArrayIndexKey: a sentence's text runs are positional
            <Fragment key={at}>
              {part.kind === "text" ? part.text : ""}
            </Fragment>
          );
        }
        return (
          <a key={part.name} href={routeHash(settingsHref(link.page))}>
            {t(link.label)}
          </a>
        );
      })}
    </p>
  );
}

const HOW_STEPS = [
  ["reviewTemplates.how.closedTitle", "reviewTemplates.how.closedBody"],
  ["reviewTemplates.how.openTitle", "reviewTemplates.how.openBody"],
  ["reviewTemplates.how.keptTitle", "reviewTemplates.how.keptBody"],
] as const;

const HOW_NOTES = [
  "reviewTemplates.how.changes",
  "reviewTemplates.how.who",
] as const;

function HowReviewsReachReps() {
  const t = useT();
  return (
    <Panel title={t("reviewTemplates.how.title")}>
      <PanelBody>
        <ol className="rt-steps">
          {HOW_STEPS.map(([title, body], index) => (
            <li key={title} className="rt-step">
              <span className="rt-step-num" aria-hidden="true">
                {index + 1}
              </span>
              <span className="t-name">{t(title)}</span>
              <p className="t-caption rt-step-body">{t(body)}</p>
            </li>
          ))}
        </ol>
      </PanelBody>
      {HOW_NOTES.map((note) => (
        <PanelRow key={note}>
          <p className="t-caption rt-note">
            <Info aria-hidden="true" />
            {t(note)}
          </p>
        </PanelRow>
      ))}
    </Panel>
  );
}

type NumberedQuestion = Readonly<{
  position: number;
  question: ReviewQuestion;
}>;

function TemplatePanel({
  template,
  onEdit,
}: Readonly<{ template: ReviewTemplate; onEdit?: () => void }>) {
  const t = useT();
  const rows: NumberedQuestion[] = template.questions.map((question, i) => ({
    position: i + 1,
    question,
  }));
  return (
    <Panel
      className="rt-review"
      title={
        <span className="rt-head-meta">
          {template.label}
          {/* A badge for the eye only: the panel's landmark is named by the
            review's own label, which already says which outcome it follows. */}
          <span aria-hidden="true">
            <Badge tone={template.outcome === "won" ? "success" : "danger"}>
              {t(
                template.outcome === "won"
                  ? "outcomeReview.outcomeWon"
                  : "outcomeReview.outcomeLost",
              )}
            </Badge>
          </span>
        </span>
      }
      titleAction={
        <span className="rt-head">
          {/* Retired templates stay listed. An old review names the template it
            came from, so a reader who finds that name here learns it is no
            longer offered rather than meeting a name the product denies. */}
          {!template.active && <Badge>{t("reviewTemplates.retired")}</Badge>}
          <ReviewTemplateSummary questions={template.questions} />
          {onEdit && (
            <Button variant="ghost" onClick={onEdit}>
              {t("reviewTemplates.edit")}
            </Button>
          )}
        </span>
      }
    >
      <DataTable
        label={template.label}
        rows={rows}
        rowKey={(row) => row.question.key}
        columns={[
          {
            key: "order",
            header: t("reviewTemplates.col.order"),
            render: (row) => (
              <span className="rt-muted t-num">{row.position}</span>
            ),
          },
          {
            key: "question",
            header: t("reviewTemplates.question"),
            grow: true,
            render: (row) => <QuestionCell row={row} />,
          },
          {
            key: "answer",
            header: t("reviewTemplates.col.answer"),
            render: (row) => <AnswerType question={row.question} />,
          },
          {
            key: "needed",
            header: t("reviewTemplates.col.needed"),
            render: (row) => <AnswerNeeded question={row.question} />,
          },
        ]}
      />
    </Panel>
  );
}

function AnswerType({ question }: Readonly<{ question: ReviewQuestion }>) {
  const t = useT();
  return t(
    question.type === "multiselect"
      ? "reviewTemplates.type.multiselect"
      : "reviewTemplates.type.text",
  );
}

function AnswerNeeded({ question }: Readonly<{ question: ReviewQuestion }>) {
  const t = useT();
  return question.required ? (
    <Badge>{t("reviewTemplates.requiredBadge")}</Badge>
  ) : (
    <span className="rt-muted">{t("reviewTemplates.optional")}</span>
  );
}

// The close reason lands in the first question by position (firstAnswer in
// deal360/closereviewoffer.tsx), so only that row says so. On a narrow panel
// the answer columns fold into this cell (reviewtemplates.css).
function QuestionCell({ row }: Readonly<{ row: NumberedQuestion }>) {
  const t = useT();
  return (
    <CellStack>
      <span className="t-name">{row.question.label}</span>
      {row.position === 1 && (
        <span className="t-caption">{t("reviewTemplates.prefilled")}</span>
      )}
      <span className="rt-q-compact">
        <AnswerType question={row.question} />
        <AnswerNeeded question={row.question} />
      </span>
    </CellStack>
  );
}
