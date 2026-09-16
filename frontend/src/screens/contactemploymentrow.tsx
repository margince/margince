import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { Button, OverflowMenu } from "../design-system/atoms";
import { InlineText } from "../design-system/inlinetext";
import { formatDateAbbrev } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import { problemMessageOf } from "./common";
import type { Employment, EmploymentActions } from "./contactemployers";
import {
  EmploymentLogo,
  employerFacts,
  hasEmployerFacts,
  useEmployerSummary,
} from "./contactemployersummary";
import { stillHeld } from "./employmentcurrency";

// One employment edge: the company it names, the role at that company (inline-
// editable — this is the ONE place a per-company title is corrected;
// `contact.title` is a different field, edited in Details above), the dates,
// and the row's own verbs folded behind an OverflowMenu — this row already
// carries a focusable inline-edit control, so the verbs stay out of the way
// until the row is hovered or that control (or the trigger itself) has
// focus, the same reveal the company page's task rows use for theirs.
export function EmploymentRow({
  employment,
  canEdit,
  readOnlyReason,
  actions,
  onRemove,
  onEdit,
  showCompany = true,
  fallbackRole,
}: Readonly<{
  employment: Employment;
  showCompany?: boolean;
  fallbackRole?: string;
  canEdit: boolean;
  readOnlyReason: string | undefined;
  actions: EmploymentActions;
  onRemove: () => void;
  onEdit: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const detail = employmentDetail(employment, t, locale, zone);
  const summary = useEmployerSummary(employment.company_id);
  const company = summary.data;
  const hasFacts = showCompany && hasEmployerFacts(company);
  const ending =
    actions.end.isPending &&
    actions.end.variables?.relationship_id === employment.relationship_id;
  // A settled mutation is no longer pending. Match errors by relationship
  // so only the failed row keeps its message after the mutation settles.
  const endFailed =
    actions.end.isError &&
    actions.end.variables?.relationship_id === employment.relationship_id;
  return (
    <div className="pe-employment">
      <EmploymentLogo
        employment={employment}
        company={company}
        show={showCompany}
        t={t}
      />
      <span className="pe-employment-body">
        <span className="pe-employment-company">
          {showCompany &&
            (employment.company_name ? (
              <button
                type="button"
                className="pe-meta-link"
                onClick={() =>
                  navigate({
                    screen: "companies",
                    id: employment.company_id,
                  })
                }
              >
                {employment.company_name}
              </button>
            ) : (
              <span className="inlinetext">{t("field.unset")}</span>
            ))}
          {stillHeld(employment) && (
            <span className="pe-rail-value-good">{t("rel.current")}</span>
          )}
          {!stillHeld(employment) && (
            <span className="t-caption">
              {t(
                employment.employment_status === "unknown"
                  ? "employment.status.unknown"
                  : "employment.status.former",
              )}
            </span>
          )}
        </span>
        {hasFacts && company && (
          <span className="pe-employment-facts t-caption">
            {employerFacts(company, t, locale)}
          </span>
        )}
        <span className="pe-employment-role">
          <InlineText
            label={t("rel.role")}
            value={employment.role ?? ""}
            // The contact's own title stands in for a role nobody has
            // recorded on THIS employment yet: as `placeholder`, not
            // `value`, so it reads the way InlineText already draws an
            // unset field (muted) rather than as a fact this edge has
            // actually stored. The ordinary "Add title" invitation only
            // applies when there is nothing else to suggest.
            placeholder={t("field.addTitle")}
            suggested={fallbackRole}
            canEdit={canEdit}
            readOnlyReason={readOnlyReason}
            onSave={(next) =>
              actions.update.mutateAsync({
                employment,
                body: { role: next || null },
              })
            }
          />
        </span>
        {detail && (
          <span className="pe-colleague-proof t-caption">{detail}</span>
        )}
      </span>
      {canEdit && (
        <span className="pe-employment-actions">
          <OverflowMenu label={t("record.moreActions")}>
            <Button small onClick={onEdit}>
              {t("employment.edit")}
            </Button>
            {stillHeld(employment) && (
              <Button
                small
                disabled={ending}
                onClick={() => actions.end.mutate(employment)}
              >
                {t("contact.rail.markEnded")}
              </Button>
            )}
            <Button small variant="danger" onClick={onRemove}>
              {t("rel.remove")}
            </Button>
          </OverflowMenu>
        </span>
      )}
      {endFailed && (
        <p className="pe-colleague-proof t-caption" role="alert">
          {problemMessageOf(actions.end.error, t)}
        </p>
      )}
    </div>
  );
}
function employmentDetail(
  employment: Employment,
  t: ReturnType<typeof useT>,
  locale: Locale,
  zone: string,
): string {
  // Career dates keep the year and the precision actually recorded.
  const start = employment.started_at
    ? employment.started_precision === "month"
      ? employment.started_at.slice(0, 7)
      : formatDateAbbrev(employment.started_at.slice(0, 10), locale, zone)
    : undefined;
  const end = employment.ended_at
    ? employment.ended_precision === "month"
      ? employment.ended_at.slice(0, 7)
      : formatDateAbbrev(employment.ended_at.slice(0, 10), locale, zone)
    : undefined;
  if (start && end) {
    return `${start} – ${end}`;
  }
  if (end) {
    return t("rel.endedOn", { when: end });
  }
  if (start) {
    return `${start} – ${t(stillHeld(employment) ? "employment.status.current" : employment.employment_status === "unknown" ? "employment.status.unknown" : "employment.status.former")}`;
  }
  return "";
}
