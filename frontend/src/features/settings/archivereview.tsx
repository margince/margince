// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { useState } from "react";
import { api, FIRST_PAGE } from "../../api/client";
import type { components } from "../../api/schema";
import { Badge, Button, EmptyState, Field } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { CellStack } from "../../design-system/cellstack";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { DateInput, isoOrBlank } from "../../design-system/dateinput";
import { FilterBar } from "../../design-system/filterbar";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { LoadMoreButton, QueryStates } from "../../design-system/querystates";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../../design-system/recordpicker";
import { activityDay } from "../../format/activitywhen";
import { formatNumber } from "../../format/format";
import { startOfDayInZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCan, useCanWrite } from "../../lib/capability";
import { labelFor } from "../../lib/labelfor";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { useViewerZone } from "../../lib/viewerzone";
import { humanizeToken } from "./audit";

// Settings → Privacy → Retention: the archive, for the controller to delete
// what no longer serves its purpose. The server decides what a deletion keeps;
// this screen only shows that answer.

type ArchivedActivity = components["schemas"]["ArchivedActivity"];
type KeptReason = components["schemas"]["ArchiveKeptReason"];
type Deletion = components["schemas"]["ArchiveDeletionResult"];

export const ARCHIVE_KEY = ["retention", "archive"] as const;

// The server takes at most this many records per request.
const DELETION_BATCH = 100;

const KEPT_LABEL: Readonly<Record<KeptReason, MessageKey>> = {
  legal_hold: "archiveReview.kept.legalHold",
  opt_out_record: "archiveReview.kept.optOutRecord",
  kept_by_law: "archiveReview.kept.byLaw",
  not_archived: "archiveReview.kept.notArchived",
  not_found: "archiveReview.kept.notFound",
};

type Filters = Readonly<{
  before: string;
  contact: RecordPickerCandidate | null;
  company: RecordPickerCandidate | null;
}>;

const NO_FILTERS: Filters = { before: "", contact: null, company: null };

async function searchContacts(q: string): Promise<RecordPickerCandidate[]> {
  const data = unwrap(
    await api.GET("/contacts", { params: { query: { q, limit: 10 } } }),
  );
  return data.data.map((contact) => ({
    id: contact.id,
    name: contact.full_name,
  }));
}

async function searchCompanies(q: string): Promise<RecordPickerCandidate[]> {
  const data = unwrap(
    await api.GET("/companies", { params: { query: { q, limit: 10 } } }),
  );
  return data.data.map((company) => ({
    id: company.id,
    name: company.display_name,
  }));
}

/** Deletes in requests the server accepts, and adds up their answers. */
async function deleteArchived(activityIds: readonly string[]) {
  const total: Deletion = { deleted: [], kept: [] };
  for (let at = 0; at < activityIds.length; at += DELETION_BATCH) {
    const part = unwrap(
      await api.POST("/retention/archive/delete", {
        body: { activity_ids: activityIds.slice(at, at + DELETION_BATCH) },
      }),
    );
    total.deleted.push(...part.deleted);
    total.kept.push(...part.kept);
  }
  return total;
}

export function ArchiveReviewCard() {
  const t = useT();
  const canRead = useCan("retention_policy", "read");
  if (!canRead) {
    return null;
  }
  return (
    <Panel title={t("archiveReview.title")}>
      <ArchiveReview />
    </Panel>
  );
}

function ArchiveReview() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const tz = useViewerZone();
  const queryClient = useQueryClient();
  const canDelete = useCanWrite("retention_policy", "delete");
  const [filters, setFilters] = useState<Filters>(NO_FILTERS);
  const [confirming, setConfirming] = useState(false);
  const before = filters.before
    ? startOfDayInZone(filters.before, tz)
    : undefined;
  const query = useInfiniteQuery({
    queryKey: [
      ...ARCHIVE_KEY,
      before,
      filters.contact?.id,
      filters.company?.id,
    ],
    initialPageParam: FIRST_PAGE,
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api.GET("/retention/archive", {
          params: {
            query: {
              before,
              contact_id: filters.contact?.id,
              company_id: filters.company?.id,
              cursor: pageParam ?? undefined,
            },
          },
        }),
      ),
    getNextPageParam: (last) => last.page.next_cursor ?? null,
  });
  const remove = useMutation({
    mutationFn: deleteArchived,
    onSuccess: () => {
      setConfirming(false);
      return queryClient.invalidateQueries({ queryKey: ARCHIVE_KEY });
    },
  });
  const rows = query.data?.pages.flatMap((page) => page.data) ?? [];
  const deletable = rows
    .filter((row) => row.kept === undefined)
    .map((row) => row.activity_id);

  const columns: DataTableColumn<ArchivedActivity>[] = [
    {
      key: "record",
      header: t("archiveReview.record"),
      grow: true,
      render: (row) => (
        <CellStack>
          <span>{row.subject ?? humanizeToken(row.kind)}</span>
          <span className="t-caption">
            {activityDay(row.occurred_at, row.occurred_precision, locale, tz)}
          </span>
        </CellStack>
      ),
    },
    {
      key: "with",
      header: t("archiveReview.with"),
      render: (row) =>
        [...row.contacts, ...row.companies].map((p) => p.name).join(", ") ||
        t("archiveReview.withNobody"),
    },
    {
      key: "kept",
      header: t("archiveReview.status"),
      render: (row) =>
        row.kept ? (
          <Badge tone="warning">{t(labelFor(KEPT_LABEL, row.kept))}</Badge>
        ) : (
          t("archiveReview.deletable")
        ),
    },
  ];

  return (
    <>
      <PanelBody className="form-stack">
        <PanelIntro>{t("archiveReview.intro")}</PanelIntro>
        <FilterBar
          label={t("archiveReview.filters")}
          quiet
          actions={
            canDelete && deletable.length > 0 ? (
              <Button
                variant="danger"
                aria-haspopup="dialog"
                onClick={() => setConfirming(true)}
              >
                {plural("archiveReview.deleteShown", deletable.length, {
                  count: formatNumber(deletable.length, locale),
                })}
              </Button>
            ) : null
          }
        >
          <Field label={t("archiveReview.before")}>
            {(field) => (
              <DateInput
                {...field}
                value={isoOrBlank(filters.before)}
                onChange={(event) =>
                  setFilters({ ...filters, before: event.target.value })
                }
              />
            )}
          </Field>
          <Field label={t("archiveReview.contact")}>
            {(control) => (
              <RecordPicker
                id={control.id}
                aria-describedby={control["aria-describedby"]}
                searchTargets={searchContacts}
                selected={filters.contact}
                onPick={(contact) => setFilters({ ...filters, contact })}
                onSearchChange={() => setFilters({ ...filters, contact: null })}
              />
            )}
          </Field>
          <Field label={t("archiveReview.company")}>
            {(control) => (
              <RecordPicker
                id={control.id}
                aria-describedby={control["aria-describedby"]}
                searchTargets={searchCompanies}
                selected={filters.company}
                onPick={(company) => setFilters({ ...filters, company })}
                onSearchChange={() => setFilters({ ...filters, company: null })}
              />
            )}
          </Field>
        </FilterBar>
        {remove.data ? <DeletionOutcome deletion={remove.data} /> : null}
        {rows.length === 0 && (
          <QueryStates query={query} pendingLabel={t("archiveReview.title")}>
            <EmptyState>{t("archiveReview.empty")}</EmptyState>
          </QueryStates>
        )}
      </PanelBody>
      {rows.length > 0 && (
        <DataTable
          label={t("archiveReview.title")}
          bleed
          fold
          columns={columns}
          rows={rows}
          rowKey={(row) => row.activity_id}
        />
      )}
      <PanelBody>
        <LoadMoreButton query={query} />
      </PanelBody>
      <ConfirmModal
        open={confirming}
        onClose={() => {
          remove.reset();
          setConfirming(false);
        }}
        title={t("archiveReview.confirmTitle")}
        confirmLabel={t("archiveReview.confirm")}
        confirmVariant="danger"
        onConfirm={() => remove.mutate(deletable)}
        pending={remove.isPending}
        error={remove.error ? problemMessageOf(remove.error, t) : null}
      >
        <p>
          {plural("archiveReview.confirmBody", deletable.length, {
            count: formatNumber(deletable.length, locale),
          })}
        </p>
      </ConfirmModal>
    </>
  );
}

/** What the last deletion did, with each kept record's reason counted. */
function DeletionOutcome({ deletion }: Readonly<{ deletion: Deletion }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const counts = new Map<KeptReason, number>();
  for (const kept of deletion.kept) {
    counts.set(kept.reason, (counts.get(kept.reason) ?? 0) + 1);
  }
  return (
    <Callout title={t("archiveReview.doneTitle")}>
      <p>
        {plural("archiveReview.deleted", deletion.deleted.length, {
          count: formatNumber(deletion.deleted.length, locale),
        })}
      </p>
      {[...counts].map(([reason, count]) => (
        <p key={reason}>
          {plural("archiveReview.keptCount", count, {
            reason: t(labelFor(KEPT_LABEL, reason)),
            count: formatNumber(count, locale),
          })}
        </p>
      ))}
    </Callout>
  );
}
