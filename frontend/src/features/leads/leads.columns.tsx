// The lead table's columns. The company has a column of its own rather than a
// caption behind the name: a queue of hundreds of leads is worked company by
// company, so the company has to be readable and sortable, and a long name in
// front of it can no longer push it off the row.
import type { components } from "../../api/schema";
import {
  createdColumn,
  lastActivityColumn,
  ownerColumn,
  tagsColumn,
} from "../../components/recordlist";
import { Badge } from "../../design-system/atoms";
import { CellStrip, type ListColumn } from "../../design-system/listtable";
import { formatDateAbbrev, formatNumber } from "../../format/format";
import { leadIdentityName } from "../../format/leadname";
import type { useLocale, useT } from "../../i18n";
import { type LeadOrigin, originLabelFor } from "../../lib/leadorigins";
import { LeadCompanyName } from "./leadcompany";
import { openTaskCountLabel } from "./leadopentasks";
import {
  SlaBadge,
  StatusBadge,
  scoreFactorLabel,
  scoreTone,
} from "./leadpresentation";
import { terminalBadge } from "./leadstanding";

type Lead = components["schemas"]["Lead"];
type Translate = ReturnType<typeof useT>;
type Locale = ReturnType<typeof useLocale>["locale"];

export function leadColumns(
  t: Translate,
  locale: Locale,
  recordZone: string,
  origins: readonly LeadOrigin[] | undefined,
): ListColumn<Lead>[] {
  return [
    {
      key: "name",
      header: t("contacts.name"),
      cell: (lead: Lead) => {
        const terminal = terminalBadge(lead);
        return (
          <span>
            <strong>{leadIdentityName(lead) || t("lead.unnamed")}</strong>
            {terminal && (
              <Badge tone={terminal.tone}>{t(terminal.label)}</Badge>
            )}
          </span>
        );
      },
      // `full_name` is in the server's lead sort vocabulary, so the header is
      // live and the attribute joins the sort menu — which is the only place an
      // alphabetical order is offered now that no view tab spells one.
      sort: "full_name",
      fixed: true,
    },
    {
      key: "company",
      header: t("create.companyName"),
      // The full name on hover: the table ellipsizes every cell alike. A linked
      // company's link names itself, so the typed name could only contradict it.
      cell: (lead: Lead) => (
        <span
          title={lead.company_id ? undefined : (lead.company_name ?? undefined)}
        >
          <LeadCompanyName lead={lead} />
        </span>
      ),
      sort: "company_name",
    },
    {
      key: "score",
      header: t("lead.score"),
      cell: (lead: Lead) => (
        <CellStrip>
          <Badge tone={scoreTone(lead.score)}>
            {formatNumber(lead.score, locale)}
          </Badge>
          <span className="t-caption">
            {lead.score_reason
              ? scoreFactorLabel(lead.score_reason, t)
              : t("lead.scoreNoSignals")}
          </span>
        </CellStrip>
      ),
      sort: "score",
      numeric: true,
    },
    {
      key: "status",
      header: t("lead.status"),
      sort: "status",
      cell: (lead: Lead) => (
        <span className="lead-status-cell">
          <StatusBadge status={lead.status} />
          <SlaBadge state={lead.sla_state} />
        </span>
      ),
    },
    {
      key: "nextTask",
      header: t("lead.nextTask"),
      sort: "next_task_due_at", // the deadline, not the title

      cell: (lead: Lead) => (
        <span>
          {lead.next_task_subject ?? t("lead.noNextTask")}
          {lead.open_task_count
            ? ` · ${openTaskCountLabel(locale, lead.open_task_count)}`
            : ""}
          {lead.next_task_due_at
            ? ` · ${formatDateAbbrev(lead.next_task_due_at, locale, recordZone)}`
            : ""}
        </span>
      ),
    },
    // The shared column, now that this header can offer a sort.
    lastActivityColumn<Lead>(t, locale, recordZone),
    {
      key: "origin",
      header: t("lead.origin"),
      sort: "origin", // the catalog's label, which is what the cell prints
      cell: (lead: Lead) => (
        <span>{originLabelFor(lead, origins) ?? t("lead.originUnset")}</span>
      ),
    },
    tagsColumn<Lead>(t),
    ownerColumn<Lead>(t),
    createdColumn<Lead>(t, locale, recordZone),
  ];
}
