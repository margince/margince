// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Download } from "lucide-react";
import { useState } from "react";
import { Badge } from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { CountLine } from "../../design-system/listsurface";
import { fitsOnePage, PAGE_SIZES } from "../../design-system/listtable.paging";
import { Pager } from "../../design-system/pager";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import type { Locale } from "../../i18n/locale";
import { valueFor } from "../../lib/labelfor";
import { reasonText } from "./importreason";
import type { ImportProblemKind, ImportRowIssue } from "./importtypes";

/**
 * Where the server builds one run's problems as a spreadsheet, with its header
 * and its reasons in the reader's language.
 */
export function problemsHref(id: string, lang: Locale): string {
  return `/v1/imports/${encodeURIComponent(id)}/problems.csv?lang=${lang}`;
}

/** The run's problems as a spreadsheet: the full list, whatever page is shown. */
export function ProblemsDownload({ runId }: Readonly<{ runId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <a className="btn btn-ghost" href={problemsHref(runId, locale)} download>
      <Download aria-hidden />
      {t("import.result.problemsFile")}
    </a>
  );
}

const PROBLEM_TONE = {
  broken_link: "warning",
  dropped_link: "default",
  invalid_value: "danger",
  unknown_owner: "info",
  new_tag: "info",
  duplicate: "info",
  permission: "danger",
  unusable_row: "danger",
  incomplete_identity: "warning",
} as const satisfies Record<ImportProblemKind, string>;

/** Where a problem is: its row and column, or the whole file when it has no row. */
function issueWhere(
  issue: ImportRowIssue,
  t: ReturnType<typeof useT>,
  locale: Locale,
): string {
  if (issue.line === undefined) return t("import.test.wholeFile");
  const line = formatNumber(issue.line, locale);
  return issue.column
    ? t("import.test.rowColumn", { line, column: issue.column })
    : t("import.test.row", { line });
}

/** Every problem the report names, with the file, row and column it is in. */
export function ProblemTable({
  issues,
}: Readonly<{ issues: readonly ImportRowIssue[] }>) {
  const t = useT();
  const [page, setPage] = useState(1);
  const [perPage, setPerPage] = useState<number>(PAGE_SIZES[0]);
  // A new report is another list: its page one, not the page the last one was on.
  const [seen, setSeen] = useState(issues);
  if (seen !== issues) {
    setSeen(issues);
    setPage(1);
  }
  if (issues.length === 0) return null;
  const lastPage = Math.max(1, Math.ceil(issues.length / perPage));
  const current = Math.min(page, lastPage);
  const from = (current - 1) * perPage;
  const shown = issues.slice(from, from + perPage);
  const paged = !fitsOnePage(issues, undefined, false);
  return (
    <div className="import__problems">
      <ProblemRows issues={shown} total={issues.length} />
      {paged ? (
        <>
          <p className="t-caption">
            <CountLine
              unit={t("import.test.issuesUnit")}
              first={from + 1}
              last={from + shown.length}
              total={issues.length}
            />
          </p>
          <Pager
            current={current}
            lastPage={lastPage}
            hasMore={false}
            perPage={perPage}
            onGoto={setPage}
            onPerPage={(next) => {
              setPerPage(next);
              setPage(1);
            }}
          />
        </>
      ) : null}
    </div>
  );
}

function ProblemRows({
  issues,
  total,
}: Readonly<{ issues: readonly ImportRowIssue[]; total: number }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <DataTable<ImportRowIssue>
      label={plural("import.test.problems", total, {
        count: formatNumber(total, locale),
      })}
      rows={[...issues]}
      rowKey={(issue) =>
        `${issue.file ?? ""}-${issue.line ?? ""}-${issue.column ?? ""}-${issue.kind ?? ""}-${issue.value ?? ""}`
      }
      columns={[
        {
          key: "where",
          header: t("import.test.col.where"),
          render: (issue) => (
            <>
              {issue.file ? <code>{issue.file}</code> : null}
              <div className="t-caption">{issueWhere(issue, t, locale)}</div>
            </>
          ),
        },
        {
          key: "problem",
          header: t("import.test.col.problem"),
          grow: true,
          render: (issue) => (
            <>
              {issue.kind ? (
                <Badge tone={valueFor(PROBLEM_TONE, issue.kind, "default")}>
                  {t(`import.problem.${issue.kind}`)}
                </Badge>
              ) : null}
              <div className="t-sub">
                {reasonText(t, issue.reason, issue.reason_code, issue.params)}
              </div>
            </>
          ),
        },
        {
          key: "value",
          header: t("import.test.col.value"),
          render: (issue) => (issue.value ? <code>{issue.value}</code> : null),
        },
      ]}
    />
  );
}
