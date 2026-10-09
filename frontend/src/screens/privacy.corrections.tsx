// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { useState } from "react";

import { api, FIRST_PAGE } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import {
  Badge,
  Button,
  EmptyState,
  Field,
  Textarea,
} from "../design-system/atoms";
import { CellStack } from "../design-system/cellstack";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { formatDate } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import {
  LoadMoreButton,
  problemMessageOf,
  QueryStates,
  throwProblem,
} from "./common";
import { correctableFieldLabel } from "./confirmfields";
import { EntityRef } from "./entityref";

type ConfirmSubmission = components["schemas"]["ConfirmSubmission"];
type Resolution = "accepted" | "rejected";

// A correction is only reviewable as a comparison ("she says Schmidt, we hold
// Schmitt"), so a row shows the proposal beside what the record holds now.
export function ConfirmSubmissionsPanel() {
  const t = useT();
  const queryClient = useQueryClient();
  // `useCanWrite`, not `useCan`: the seat ceiling refuses a read seat's POST
  // even when it holds the grant.
  const canDecide = useCanWrite("contact", "update");
  const [deciding, setDeciding] = useState<ConfirmSubmission | null>(null);
  const [note, setNote] = useState("");

  // Paged, because the queue is as long as the contacts make it.
  const query = useInfiniteQuery({
    queryKey: ["confirm-submissions"],
    initialPageParam: FIRST_PAGE,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await api.GET("/confirm-submissions", {
        params: { query: { resolved: false, cursor: pageParam ?? undefined } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    getNextPageParam: (last) => last.page.next_cursor ?? null,
  });

  const resolve = useMutation({
    mutationFn: async (command: {
      id: string;
      resolution: Resolution;
      note: string;
    }) => {
      const { data, error } = await api.POST(
        "/confirm-submissions/{id}/resolve",
        {
          params: { path: { id: command.id } },
          body: {
            resolution: command.resolution,
            note: command.note || undefined,
          },
        },
      );
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: async () => {
      setDeciding(null);
      setNote("");
      await queryClient.invalidateQueries({
        queryKey: ["confirm-submissions"],
      });
      // An accepted correction wrote a field the contact's own page shows.
      await queryClient.invalidateQueries({ queryKey: ["contact"] });
    },
  });

  const rows = query.data?.pages.flatMap((page) => page.data) ?? [];
  return (
    <Panel title={t("privacy.corrections")}>
      <PanelBody>
        <PanelIntro>{t("privacy.correctionsSub")}</PanelIntro>
      </PanelBody>
      {query.isSuccess && rows.length > 0 ? (
        <>
          <CorrectionTable
            rows={rows}
            canDecide={canDecide}
            onDecide={(row) => {
              resolve.reset();
              setNote("");
              setDeciding(row);
            }}
          />
          {query.hasNextPage && (
            <PanelBody>
              <LoadMoreButton query={query} />
            </PanelBody>
          )}
        </>
      ) : (
        <PanelBody>
          <QueryStates query={query} pendingLabel={t("privacy.corrections")}>
            <EmptyState>{t("privacy.correctionsEmpty")}</EmptyState>
          </QueryStates>
        </PanelBody>
      )}
      <DecideModal
        row={deciding}
        note={note}
        pending={resolve.isPending}
        error={resolve.error ? problemMessageOf(resolve.error, t) : null}
        onNote={setNote}
        onClose={() => setDeciding(null)}
        onDecide={(row, resolution) =>
          resolve.mutate({ id: row.id, resolution, note })
        }
      />
    </Panel>
  );
}

function CorrectionTable({
  rows,
  canDecide,
  onDecide,
}: Readonly<{
  rows: ConfirmSubmission[];
  canDecide: boolean;
  onDecide: (row: ConfirmSubmission) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const tz = viewerZone();
  const columns: DataTableColumn<ConfirmSubmission>[] = [
    {
      key: "contact",
      header: t("privacy.correctionContact"),
      // Who, first: accepting either of two identical proposals changes a
      // different record.
      render: (row) => (
        <EntityRef
          kind="contact"
          id={row.contact_id}
          name={row.contact_name ?? t("privacy.correctionUnnamed")}
        />
      ),
    },
    {
      key: "change",
      header: t("privacy.correctionChange"),
      grow: true,
      render: (row) => <ChangeCell row={row} />,
    },
    {
      key: "received",
      header: t("privacy.correctionReceived"),
      render: (row) => formatDate(row.submitted_at, locale, tz),
    },
    {
      key: "kind",
      header: t("privacy.correctionKind"),
      render: (row) =>
        row.field ? (
          <Badge>{t("privacy.correctionKindCorrection")}</Badge>
        ) : (
          <Badge tone="warning">{t("privacy.correctionKindRemoval")}</Badge>
        ),
    },
  ];
  if (canDecide) {
    columns.push({
      key: "verbs",
      header: t("table.actions"),
      headerHidden: true,
      fold: "end",
      align: "end",
      render: (row) => (
        <span className="cell-actions">
          <Button
            aria-label={t("privacy.correctionDecideNamed", {
              name: row.contact_name ?? t("privacy.correctionUnnamed"),
            })}
            aria-haspopup="dialog"
            onClick={() => onDecide(row)}
          >
            {t("privacy.correctionDecide")}
          </Button>
        </span>
      ),
    });
  }
  return (
    <DataTable
      label={t("privacy.corrections")}
      bleed
      fold
      columns={columns}
      rows={rows}
      rowKey={(row) => row.id}
    />
  );
}

function ChangeCell({ row }: Readonly<{ row: ConfirmSubmission }>) {
  const t = useT();
  if (!row.field) {
    return <>{t("privacy.correctionRemoval")}</>;
  }
  const label = correctableFieldLabel(row.field);
  return (
    <CellStack>
      <span>{label ? t(label) : row.field}</span>
      {/* Not a catalog key: an arrow between two values has no words to translate. */}
      <span className="t-caption">
        {row.current_value
          ? `${row.current_value} → ${row.proposed_value ?? ""}`
          : row.proposed_value}
      </span>
    </CellStack>
  );
}

// The note matters most on a rejection: an accepted correction explains itself,
// and the contact who asks again is owed the reason it was not.
function DecideModal({
  row,
  note,
  pending,
  error,
  onNote,
  onClose,
  onDecide,
}: Readonly<{
  row: ConfirmSubmission | null;
  note: string;
  pending: boolean;
  error: string | null;
  onNote: (value: string) => void;
  onClose: () => void;
  onDecide: (row: ConfirmSubmission, resolution: Resolution) => void;
}>) {
  const t = useT();
  const name = row?.contact_name ?? t("privacy.correctionUnnamed");
  return (
    <ConfirmModal
      open={row !== null}
      onClose={onClose}
      title={t("privacy.correctionDialogTitle", { name })}
      // A removal is acknowledged here, not performed: the rights case opened
      // when it arrived is where it is answered.
      confirmLabel={
        row?.field
          ? t("privacy.correctionAccept")
          : t("privacy.correctionAcknowledge")
      }
      onConfirm={() => row && onDecide(row, "accepted")}
      actionsLead={
        <Button
          variant="ghost"
          disabled={pending}
          onClick={() => row && onDecide(row, "rejected")}
        >
          {t("privacy.correctionReject")}
        </Button>
      }
      pending={pending}
      error={error}
    >
      {row && <ChangeCell row={row} />}
      <Field label={t("privacy.correctionNote")}>
        {(control) => (
          <Textarea
            {...control}
            value={note}
            onChange={(event) => onNote(event.target.value)}
            maxLength={500}
          />
        )}
      </Field>
    </ConfirmModal>
  );
}
