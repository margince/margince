import type { ReactNode } from "react";
import {
  CompanyMark,
  employerFacts,
  hasEmployerFacts,
  useEmployerSummary,
} from "../../components/contactemployersummary";
import { Button, OverflowMenu } from "../../design-system/atoms";
import { CompanyCard } from "../../design-system/companycard";
import { ErrorLine } from "../../design-system/errorline";
import { InlineText } from "../../design-system/inlinetext";
import { formatDateAbbrev } from "../../format/format";
import { type Locale, useLocale, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { navigate } from "../../lib/router";
import type { Employment, EmploymentActions } from "./contactemployers";
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
  mark,
}: Readonly<{
  employment: Employment;
  showCompany?: boolean;
  // The row's provenance mark, drawn beside the company button, never inside it.
  mark?: ReactNode;
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
    <CompanyCard
      // Only the first row naming a company carries its mark; the slot stays
      // so every row's name and role line up under one another.
      logo={
        showCompany && (
          <CompanyMark
            companyId={employment.company_id}
            name={employment.company_name ?? t("field.unset")}
            logoUrl={company?.logo_url}
          />
        )
      }
      title={showCompany && <EmployerName employment={employment} />}
      badges={
        <>
          {mark}
          <EmploymentStatus employment={employment} />
        </>
      }
      facts={hasFacts && company ? employerFacts(company, t, locale) : null}
      actions={
        canEdit && (
          <OverflowMenu label={t("record.moreActions")}>
            <Button onClick={onEdit}>{t("employment.edit")}</Button>
            {stillHeld(employment) && (
              <Button
                disabled={ending}
                onClick={() => actions.end.mutate(employment)}
              >
                {t("contact.rail.markEnded")}
              </Button>
            )}
            <Button variant="danger" onClick={onRemove}>
              {t("rel.remove")}
            </Button>
          </OverflowMenu>
        )
      }
    >
      <span className="pe-employment-role">
        <InlineText
          label={t("rel.role")}
          value={employment.role ?? ""}
          // The contact's own title stands in for a role nobody has recorded
          // on this employment yet. It goes in `suggested`, so it reads as
          // InlineText draws an unset field (muted) and not as a stored fact.
          // The ordinary "Add title" invitation applies only when there is
          // nothing else to suggest.
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
      {detail && <span className="pe-colleague-proof t-caption">{detail}</span>}
      {endFailed && <ErrorLine error={actions.end.error} />}
    </CompanyCard>
  );
}
function EmployerName({ employment }: Readonly<{ employment: Employment }>) {
  const t = useT();
  return employment.company_name ? (
    <button
      type="button"
      onClick={() =>
        navigate({ screen: "companies", id: employment.company_id })
      }
    >
      {employment.company_name}
    </button>
  ) : (
    <span className="inlinetext">{t("field.unset")}</span>
  );
}

function EmploymentStatus({
  employment,
}: Readonly<{ employment: Employment }>) {
  const t = useT();
  if (stillHeld(employment)) {
    return <span className="pe-rail-value-good">{t("rel.current")}</span>;
  }
  return (
    <span className="t-caption">
      {t(
        employment.employment_status === "unknown"
          ? "employment.status.unknown"
          : "employment.status.former",
      )}
    </span>
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
