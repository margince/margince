// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { requireVersion } from "../api/version";
import { Button } from "../design-system/atoms";
import { CellStack } from "../design-system/cellstack";
import { ErrorLine } from "../design-system/errorline";
import { useRowSelection } from "../design-system/listselection";
import { ListTable } from "../design-system/listtable";
import { QueryStates } from "../design-system/querystates";
import { RecordFormDialog } from "../design-system/recordformdialog";
import { formatDateTime } from "../format/format";
import { type Translator, useLocale, useT } from "../i18n";
import { useOpenSource } from "../lib/opensource";
import { unwrap } from "../lib/problem";
import { useViewerZone } from "../lib/viewerzone";
import { inquiryReasonKey } from "../lib/websiteinquiries";
import { invalidateHandledChange } from "../lib/workkeys";
import { EntityRef } from "./entityref";

type Activity = components["schemas"]["Activity"];
type Automation = components["schemas"]["Automation"];
type Preview = components["schemas"]["WebsiteInquiryPreviewItem"];
type Result = components["schemas"]["WebsiteInquiryHandlingItem"];

const firstSourceCursor: string | null = null;

export function WebsiteInquiryExistingDialog({
  form,
  automation,
  onClose,
}: Readonly<{
  form: components["schemas"]["WebForm"];
  automation: Automation;
  onClose: () => void;
}>) {
  const t = useT();
  const client = useQueryClient();
  const viewerZone = useViewerZone();
  const [reviewed, setReviewed] = useState<readonly Activity[] | null>(null);
  const listKey = ["website-inquiry-sources", form.id];
  const list = useInfiniteQuery({
    queryKey: listKey,
    initialPageParam: firstSourceCursor,
    gcTime: 0,
    queryFn: async ({ pageParam }: { pageParam: string | null }) =>
      unwrap(
        await api.GET("/activities", {
          params: {
            query: {
              website_form_id: form.id,
              limit: 50,
              ...(pageParam ? { cursor: pageParam } : {}),
            },
          },
        }),
      ),
    getNextPageParam: (last) =>
      last.page.has_more ? last.page.next_cursor : undefined,
  });
  const rows = list.data?.pages.flatMap((page) => page.data) ?? [];
  const selection = useRowSelection({
    rows,
    keyOf: (row) => row.id,
    limit: 50,
  });
  const preview = useQuery({
    queryKey: [
      "website-inquiry-selected-preview",
      form.id,
      automation.id,
      automation.version,
      reviewed,
    ],
    enabled: !!reviewed,
    retry: false,
    gcTime: 0,
    refetchOnWindowFocus: false,
    queryFn: async () => {
      if (!automation.inquiry_policy)
        throw new Error(t("inquiry.policyUnavailable"));
      return unwrap(
        await api.POST("/web-forms/{id}/handling/preview", {
          params: { path: { id: form.id } },
          body: {
            policy: automation.inquiry_policy,
            automation_id: automation.id,
            automation_version: requireVersion(automation.version),
            selected: (reviewed ?? []).map(sourceSelection),
          },
        }),
      );
    },
  });
  const handle = useMutation({
    mutationFn: async (
      request: Readonly<{
        formId: string;
        automationId: string;
        version: number;
        selected: components["schemas"]["WebsiteInquirySelection"][];
      }>,
    ) =>
      unwrap(
        await api.POST("/web-forms/{id}/handling/existing", {
          params: { path: { id: request.formId } },
          body: {
            automation_id: request.automationId,
            automation_version: request.version,
            selected: request.selected,
          },
        }),
      ),
    onSuccess: async () => {
      await invalidateHandledChange(client);
      await client.invalidateQueries({ queryKey: ["automations"] });
    },
  });
  const eligible = new Set(
    preview.data?.items
      .filter((item) => item.status === "eligible")
      .map((item) => item.id),
  );
  const results = handle.data?.items ?? preview.data?.items;
  const restart = async () => {
    await client.invalidateQueries({ queryKey: listKey });
    handle.reset();
    setReviewed(null);
    selection.clear();
  };
  return (
    <RecordFormDialog
      open
      title={t("inquiry.existing")}
      fieldCount={8}
      onClose={onClose}
      closeDisabled={handle.isPending}
      form={
        <div className="form-stack">
          <p>{t("inquiry.existingScope", { name: form.name })}</p>
          <p>{t("inquiry.existingLimit")}</p>
          {reviewed ? (
            <QueryStates
              query={preview}
              pendingLabel={t("inquiry.reviewSelection")}
            >
              <InquirySelectionResults
                sources={reviewed}
                items={results ?? []}
                zone={automation.inquiry_policy?.timezone ?? viewerZone}
              />
            </QueryStates>
          ) : (
            <QueryStates
              query={list}
              pendingLabel={t("inquiry.loadingSources")}
            >
              <InquirySourceTable
                rows={rows}
                selection={selection}
                hasMore={list.hasNextPage}
                loadMore={() => list.fetchNextPage()}
                pending={list.isFetchingNextPage}
              />
            </QueryStates>
          )}
          <ErrorLine error={handle.error} />
        </div>
      }
      actions={
        <>
          <Button disabled={handle.isPending} onClick={onClose}>
            {t("common.close")}
          </Button>
          {reviewed ? (
            <>
              <Button disabled={handle.isPending} onClick={restart}>
                {t("inquiry.chooseAgain")}
              </Button>
              {!handle.data && (
                <Button
                  variant="primary"
                  pending={handle.isPending}
                  disabled={preview.isFetching || !eligible.size}
                  onClick={() =>
                    handle.mutate({
                      formId: form.id,
                      automationId: automation.id,
                      version: requireVersion(automation.version),
                      selected: reviewed
                        .filter((source) => eligible.has(source.id))
                        .map(sourceSelection),
                    })
                  }
                >
                  {t("inquiry.handleSelected")}
                </Button>
              )}
            </>
          ) : (
            <Button
              variant="primary"
              disabled={!selection.selected.size}
              onClick={() => setReviewed(selection.selectedRows)}
            >
              {t("inquiry.reviewSelection")}
            </Button>
          )}
        </>
      }
    />
  );
}

function sourceSelection(
  source: Activity,
): components["schemas"]["WebsiteInquirySelection"] {
  return { id: source.id, version: requireVersion(source.version) };
}

function InquirySourceTable({
  rows,
  selection,
  hasMore,
  loadMore,
  pending,
}: Readonly<{
  rows: Activity[];
  selection: ReturnType<typeof useRowSelection<Activity>>;
  hasMore: boolean;
  loadMore: () => void;
  pending: boolean;
}>) {
  const t = useT();
  const open = useOpenSource();
  return (
    <ListTable
      rows={rows}
      rowKey={(row) => row.id}
      unit={t("inquiry.submissions")}
      hasMore={hasMore}
      onLoadMore={loadMore}
      pending={pending}
      selection={{
        ...selection,
        label: (row) =>
          t("inquiry.selectSource", { subject: senderOf(row, t) }),
        selectable: (row) => !!row.version,
        bar: <p>{t("inquiry.existingLimit")}</p>,
      }}
      columns={[
        {
          key: "source",
          header: t("inquiry.submission"),
          fixed: true,
          cell: (row) => <InquirySource source={row} open={open} />,
        },
        {
          key: "received",
          header: t("inquiry.received"),
          cell: (row) => <InquiryDate value={row.created_at} />,
        },
      ]}
    />
  );
}

// Every note from one form carries the same subject, so a row is named by
// who sent it. A note whose fields are withheld or were never kept keeps it.
function senderOf(source: Activity, t: Translator): string {
  const sent = source.web_form;
  return (
    sent?.full_name || sent?.email || source.subject || t("inquiry.untitled")
  );
}

function InquirySource({
  source,
  open,
}: Readonly<{ source: Activity; open: ReturnType<typeof useOpenSource> }>) {
  const t = useT();
  const name = senderOf(source, t);
  const sent = source.web_form;
  const email = sent?.full_name ? sent.email : "";
  const opening = firstLine(sent?.message ?? "");
  return (
    <CellStack>
      {open ? (
        <Button variant="link" onClick={() => open(source.id)}>
          {name}
        </Button>
      ) : (
        <span>{name}</span>
      )}
      {email && <span className="t-caption">{email}</span>}
      {opening && <span className="t-caption">{opening}</span>}
    </CellStack>
  );
}

// The visitor's own text, so it is read as plain lines and never as markdown.
function firstLine(message: string): string {
  return (
    message
      .split(/\r?\n/)
      .map((line) => line.trim())
      .find(Boolean) ?? ""
  );
}

function InquiryDate({
  value,
  zone,
}: Readonly<{ value: string; zone?: string }>) {
  const { locale } = useLocale();
  const viewerZone = useViewerZone();
  return formatDateTime(value, locale, zone ?? viewerZone);
}

function InquirySelectionResults({
  sources,
  items,
  zone,
}: Readonly<{
  sources: readonly Activity[];
  items: readonly (Preview | Result)[];
  zone: string;
}>) {
  const t = useT();
  const open = useOpenSource();
  return (
    <ListTable
      rows={items}
      rowKey={(row) => row.id}
      unit={t("inquiry.submissions")}
      columns={[
        {
          key: "source",
          header: t("inquiry.submission"),
          fixed: true,
          cell: (row) => {
            const source = sources.find((each) => each.id === row.id);
            return source ? senderOf(source, t) : t("inquiry.untitled");
          },
        },
        {
          key: "status",
          header: t("inquiry.result"),
          cell: (row) => inquiryResultText(row.status, t),
        },
        {
          key: "recipient",
          header: t("inquiry.colleague"),
          cell: (row) =>
            "recipient_id" in row && row.recipient_id ? (
              <EntityRef kind="user" id={row.recipient_id} />
            ) : null,
        },
        {
          key: "record",
          header: t("inquiry.record"),
          cell: (row) =>
            "target_type" in row &&
            row.target_type &&
            row.target_id &&
            (row.target_type === "lead" || row.target_type === "contact") ? (
              <EntityRef kind={row.target_type} id={row.target_id} />
            ) : null,
        },
        {
          key: "reason",
          header: t("inquiry.assignmentReason"),
          cell: (row) => (row.reason ? t(inquiryReasonKey(row.reason)) : null),
        },
        {
          key: "due",
          header: t("inquiry.deadline"),
          cell: (row) =>
            "due_at" in row && row.due_at ? (
              <InquiryDate value={row.due_at} zone={zone} />
            ) : null,
        },
        {
          key: "task",
          header: t("inquiry.task"),
          cell: (row) =>
            "task_id" in row && row.task_id && open ? (
              <Button
                variant="link"
                onClick={() => row.task_id && open(row.task_id)}
              >
                {t("inquiry.openTask")}
              </Button>
            ) : null,
        },
      ]}
    />
  );
}

function inquiryResultText(status: string, t: ReturnType<typeof useT>): string {
  switch (status) {
    case "eligible":
      return t("inquiry.result.eligible");
    case "already_handled":
      return t("inquiry.result.alreadyHandled");
    case "task_created":
      return t("inquiry.result.taskCreated");
    case "waiting_for_approval":
      return t("inquiry.result.waiting");
    case "changed":
      return t("inquiry.result.changed");
    case "unavailable":
      return t("inquiry.result.unavailable");
    case "skipped":
      return t("inquiry.result.skipped");
    default:
      return t("inquiry.result.failed");
  }
}
