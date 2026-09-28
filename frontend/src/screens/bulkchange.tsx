// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  skipToken,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { type ReactNode, useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { requireVersion } from "../api/version";
import { Badge, Button, Modal, PendingBody } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { DataTable } from "../design-system/datatable";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { useToast } from "../design-system/toast";
import { formatNumber } from "../format/format";
import { type PluralBase, useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { dealRecordKeys, derivedRecordKeys } from "./activitykeys";
import { throwProblem } from "./common";
import { OwnerName } from "./entityref";

type BulkRecordType = components["schemas"]["BulkRecordType"];
type BulkVerb = components["schemas"]["BulkVerb"];
type BulkChangePreview = components["schemas"]["BulkChangePreview"];
type BulkSampleRow = components["schemas"]["BulkSampleRow"];
type BulkSkip = components["schemas"]["BulkSkip"];
type BulkSkipReason = components["schemas"]["BulkSkipReason"];
export type BulkChangeResult = components["schemas"]["BulkChangeResult"];

/** One selected row as its list holds it: the version it was shown, and its name. */
export type BulkRow = Readonly<{ id: string; version?: number; label: string }>;

/**
 * One press of a bulk verb. `openId` is minted per press, so a second press over
 * the same rows asks the server again: a confirm token is good for one run. It
 * is also the execution's Idempotency-Key, so every retry of Confirm for this
 * preview replays the first answer instead of running the change twice.
 */
export type BulkChangeRequest = Readonly<{
  recordType: BulkRecordType;
  verb: BulkVerb;
  rows: readonly BulkRow[];
  ownerId?: string;
  openId: string;
}>;

type Translate = ReturnType<typeof useT>;

type RecordKind = Readonly<{
  list: string;
  record: string;
  unit: MessageKey;
  done: PluralBase;
}>;

const RECORD_KINDS: Readonly<Record<BulkRecordType, RecordKind>> = {
  contact: {
    list: "contacts",
    record: "contact",
    unit: "unit.contacts",
    done: "bulk.doneContacts",
  },
  company: {
    list: "companies",
    record: "company",
    unit: "unit.companies",
    done: "bulk.doneCompanies",
  },
  deal: {
    list: "deals",
    record: "deal",
    unit: "unit.deals",
    done: "bulk.doneDeals",
  },
};

const SKIP_REASONS: Readonly<Record<BulkSkipReason, MessageKey>> = {
  not_found: "bulk.reason.not_found",
  not_writable: "bulk.reason.not_writable",
  changed_since_preview: "bulk.reason.changed_since_preview",
  no_change: "bulk.reason.no_change",
  anchor_company: "bulk.reason.anchor_company",
  not_previewed: "bulk.reason.not_previewed",
  refused: "bulk.reason.refused",
};

// The single-record rules a `refused` skip names by code. A code missing here
// falls back to the server's English `message`.
const REFUSAL_CODES: Readonly<Record<string, MessageKey>> = {
  sole_project_company: "bulk.refusal.sole_project_company",
  locked: "bulk.refusal.locked",
  anchor_protected: "bulk.refusal.anchor_protected",
  required: "bulk.refusal.required",
};

// The one request body both halves send. The preview and the execute must name
// the same selection, verb and owner, or the server refuses the confirm token.
function bulkBody(request: BulkChangeRequest) {
  return {
    record_type: request.recordType,
    verb: request.verb,
    items: request.rows.map((row) => ({
      id: row.id,
      version: requireVersion(row.version),
    })),
    owner_id: request.verb === "reassign_owner" ? request.ownerId : undefined,
  };
}

async function previewBulkChange(
  request: BulkChangeRequest,
  t: Translate,
): Promise<BulkChangePreview> {
  const { data, error } = await api.POST("/bulk/preview", {
    body: bulkBody(request),
  });
  if (error) {
    throwProblem(error, t);
  }
  return data;
}

type BulkRun = Readonly<{
  request: BulkChangeRequest;
  confirmToken?: string;
  idempotencyKey: string;
}>;

async function executeBulkChange(
  run: BulkRun,
  t: Translate,
): Promise<BulkChangeResult> {
  const { data, error } = await api.POST("/bulk/execute", {
    params: { header: { "Idempotency-Key": run.idempotencyKey } },
    body: { ...bulkBody(run.request), confirm_token: run.confirmToken },
  });
  if (error) {
    throwProblem(error, t);
  }
  return data;
}

function recordKeysOf(kind: RecordKind, id: string) {
  return kind.record === "deal"
    ? dealRecordKeys(id)
    : [[kind.record, id], ...derivedRecordKeys(kind.record, id)];
}

function SkipReason({ skip }: Readonly<{ skip: BulkSkip }>) {
  const t = useT();
  if (skip.reason !== "refused") {
    return <span>{t(SKIP_REASONS[skip.reason])}</span>;
  }
  const known =
    skip.code && Object.hasOwn(REFUSAL_CODES, skip.code)
      ? REFUSAL_CODES[skip.code]
      : undefined;
  if (known) {
    return <span>{t(known)}</span>;
  }
  return <span>{skip.message ?? t(SKIP_REASONS.refused)}</span>;
}

function SampleState({
  verb,
  state,
}: Readonly<{ verb: BulkVerb; state: BulkSampleRow["before"] }>) {
  const t = useT();
  if (verb === "reassign_owner") {
    return <OwnerName ownerId={state.owner_id} unowned={t("list.unowned")} />;
  }
  return state.archived ? (
    <Badge tone="warning">{t("record.archived")}</Badge>
  ) : (
    <span>{t("bulk.stateActive")}</span>
  );
}

function PreviewBody({
  request,
  preview,
}: Readonly<{ request: BulkChangeRequest; preview: BulkChangePreview }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const labels = new Map(request.rows.map((row) => [row.id, row.label]));
  return (
    <>
      {preview.count > 0 ? (
        <p>
          {plural("bulk.affects", preview.count, {
            count: formatNumber(preview.count, locale),
            total: formatNumber(request.rows.length, locale),
          })}
        </p>
      ) : (
        <p>{t("bulk.nothing")}</p>
      )}
      {preview.requires_confirmation && preview.count > 0 && (
        <Callout tone="warning" title={t("bulk.largeTitle")}>
          {t("bulk.largeBody")}
        </Callout>
      )}
      {preview.sample.length > 0 && (
        <>
          <Heading size="xsmall" as="h3">
            {t("bulk.sampleHeading")}
          </Heading>
          <DataTable
            label={t("bulk.sampleHeading")}
            rows={preview.sample}
            rowKey={(row) => row.id}
            columns={[
              {
                key: "record",
                header: t("bulk.colRecord"),
                render: (row) => row.label,
              },
              {
                key: "before",
                header: t("bulk.colNow"),
                render: (row) => (
                  <SampleState verb={preview.verb} state={row.before} />
                ),
              },
              {
                key: "after",
                header: t("bulk.colAfter"),
                render: (row) => (
                  <SampleState verb={preview.verb} state={row.after} />
                ),
              },
            ]}
          />
        </>
      )}
      {preview.excluded.length > 0 && (
        <>
          <Heading size="xsmall" as="h3">
            {plural("bulk.excluded", preview.excluded.length, {
              count: formatNumber(preview.excluded.length, locale),
            })}
          </Heading>
          <DataTable
            label={t("bulk.colReason")}
            rows={preview.excluded}
            rowKey={(skip) => skip.id}
            columns={[
              {
                key: "record",
                header: t("bulk.colRecord"),
                render: (skip) => labels.get(skip.id) ?? skip.id,
              },
              {
                key: "reason",
                header: t("bulk.colReason"),
                render: (skip) => <SkipReason skip={skip} />,
              },
            ]}
          />
        </>
      )}
    </>
  );
}

/**
 * What a bulk verb would do, asked of the server before anything is written,
 * and the one button that does it.
 *
 * A bare `Modal` rather than `ConfirmModal`: when nothing would change, or the
 * preview failed, there is nothing to confirm and the only verb is Close.
 */
export function BulkChangeDialog({
  request,
  onClose,
  onDone,
}: Readonly<{
  /** The press to preview; null while the dialog is closed. */
  request: BulkChangeRequest | null;
  onClose: () => void;
  /** Called once the change ran — the caller clears its selection. */
  onDone: (result: BulkChangeResult) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const toast = useToast();
  const queryClient = useQueryClient();
  const headingId = useId();
  // Kept after close so the dialog still has its words while it animates out.
  const [shown, setShown] = useState(request);
  if (request !== null && request !== shown) {
    setShown(request);
  }

  const preview = useQuery({
    queryKey: ["bulk-preview", shown?.openId],
    queryFn: request ? () => previewBulkChange(request, t) : skipToken,
    // Each press is its own question; a cached answer would carry a token
    // somebody may already have spent.
    gcTime: 0,
    staleTime: Number.POSITIVE_INFINITY,
    retry: false,
    refetchOnWindowFocus: false,
  });

  const execute = useMutation({
    mutationFn: (run: BulkRun) => executeBulkChange(run, t),
    onSuccess: async (result, run) => {
      const kind = RECORD_KINDS[run.request.recordType];
      for (const row of run.request.rows) {
        for (const queryKey of recordKeysOf(kind, row.id)) {
          queryClient.invalidateQueries({ queryKey });
        }
      }
      await queryClient.invalidateQueries({ queryKey: [kind.list] });
      const changed = plural(kind.done, result.changed, {
        count: formatNumber(result.changed, locale),
      });
      const skipped =
        result.skipped.length > 0
          ? plural("bulk.doneSkipped", result.skipped.length, {
              count: formatNumber(result.skipped.length, locale),
            })
          : null;
      toast.show(skipped ? `${changed} ${skipped}` : changed, {
        tone: result.changed > 0 ? "success" : "warning",
      });
      onDone(result);
    },
  });

  const close = () => {
    execute.reset();
    onClose();
  };

  if (!shown) {
    return null;
  }
  const kind = RECORD_KINDS[shown.recordType];
  const unit = t(kind.unit);
  const answer = preview.data;
  const runnable = answer !== undefined && answer.count > 0;
  const archive = shown.verb === "archive";

  let body: ReactNode;
  if (preview.isError) {
    body = <ErrorLine error={preview.error} />;
  } else if (answer === undefined) {
    body = <PendingBody label={t("bulk.checking")} />;
  } else {
    body = <PreviewBody request={shown} preview={answer} />;
  }

  return (
    <Modal open={request !== null} onClose={close} labelledBy={headingId}>
      <Heading size="large" id={headingId} className="t-h2 dialog-heading">
        {archive
          ? t("bulk.titleArchive", { unit })
          : t("bulk.titleReassign", { unit })}
      </Heading>
      <div className="form-stack">{body}</div>
      <ErrorLine error={execute.error} />
      <div className="actions">
        {runnable ? (
          <>
            <Button onClick={close} disabled={execute.isPending}>
              {t("create.cancel")}
            </Button>
            <Button
              variant={archive ? "danger" : "primary"}
              pending={execute.isPending}
              onClick={() =>
                execute.mutate({
                  request: shown,
                  confirmToken: answer.confirm_token,
                  idempotencyKey: shown.openId,
                })
              }
            >
              {archive
                ? t("bulk.confirmArchive", { unit })
                : t("bulk.confirmReassign")}
            </Button>
          </>
        ) : (
          <Button onClick={close}>{t("common.close")}</Button>
        )}
      </div>
    </Modal>
  );
}
