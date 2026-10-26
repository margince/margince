import { useInfiniteQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api, FIRST_PAGE } from "../../api/client";
import type { components } from "../../api/schema";
import { FileOrigin } from "../../components/fileorigin";
import { Badge, Button } from "../../design-system/atoms";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { FilterPills } from "../../design-system/filterpills";
import { Panel, PanelBody } from "../../design-system/panel";
import { LoadMoreButton } from "../../design-system/querystates";
import {
  type SectionState,
  SurfaceState,
} from "../../design-system/surfacestate";
import { formatDateAbbrev } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { attachmentAnchor } from "../../lib/attachmentlink";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { AddDocumentDialog } from "../voice-knowledge/adddocument";
import "./contact360.css";

// The contact's own files: unlike the sibling tabs in contacttabs.tsx, this one
// is not a read of the 360 composite — the 360 does not carry attachments —
// so it fetches its own page and classifies its own state rather than reusing
// `sectionState`, which exists for a section of a payload this tab never
// receives.
//
// It is also the one tab here that WRITES. The upload is the account library's
// own dialog (adddocument.tsx), anchored on this contact rather than on a
// company — a second upload form for the same endpoint would be a second set of
// answers about size limits, partial failures and what a category means.

type Attachment = components["schemas"]["Attachment"];
type Category = NonNullable<Attachment["category"]>;
type DocState = NonNullable<Attachment["doc_state"]>;

// The most a reader is asked to scan at once on a record page: enough to
// recognise a contact that has been busy, and the rest is one button away.
const PAGE_LIMIT = 20;

const CATEGORY_LABELS: Record<Category, MessageKey> = {
  contract: "docs.category.contract",
  offer: "docs.category.offer",
  legal: "docs.category.legal",
  email_attachment: "docs.category.email",
  message_attachment: "docs.category.message",
  other: "docs.category.other",
  link: "docs.category.link",
};

// The cuts always offered, and the two a contact's files seldom carry, which
// join the row only when a file does.
const STEADY_CUTS: readonly Category[] = [
  "offer",
  "legal",
  "email_attachment",
  "other",
];
const RARE_CUTS: readonly Category[] = [
  "message_attachment",
  "contract",
  "link",
];

const STATE_LABELS: Record<DocState, MessageKey> = {
  draft: "docs.state.draft",
  current: "docs.state.current",
  final: "docs.state.final",
  superseded: "docs.state.superseded",
};

/**
 * The contact's files as the walk has read them so far. The tab and the tab
 * strip's count share it, so the count costs no second request. `hasMore` is
 * the server's statement, read off the newest page the walk has reached, not
 * `hasNextPage`. A page that reports more rows without a cursor is still cut.
 */
export function useContactFiles(contactId: string) {
  const query = useInfiniteQuery({
    queryKey: ["attachments", "contact", contactId],
    initialPageParam: FIRST_PAGE,
    queryFn: async ({ pageParam }) => {
      return unwrap(
        await api.GET("/attachments", {
          params: {
            query: {
              entity_type: "contact",
              entity_id: contactId,
              limit: PAGE_LIMIT,
              ...(pageParam ? { cursor: pageParam } : {}),
            },
          },
        }),
      );
    },
    getNextPageParam: (last) => last.page.next_cursor ?? null,
  });
  const files = query.data?.pages.flatMap((page) => page.data) ?? [];
  const hasMore = query.data?.pages.at(-1)?.page.has_more ?? false;
  return { query, files, hasMore };
}

// Its own endpoint, so its own state: a failed read and an empty library are
// different sentences. `partial` keeps the rows read so far beside the
// sentence that says more remain. A page that fails part-way through the walk
// keeps the rows already read, and the Load more button is the retry.
function filesState(
  query: ReturnType<typeof useContactFiles>["query"],
  shown: number,
  hasMore: boolean,
): SectionState {
  if (query.isPending) return "loading";
  if (shown === 0) return query.isError ? "failed" : "empty";
  return hasMore ? "partial" : "ready";
}

export function ContactFilesTab({
  contactId,
  refusedReasonId,
  onOpenEmail,
}: Readonly<{
  contactId: string;
  /** Opens a mail in the page's drawer, from the line naming a file's mail. */
  onOpenEmail?: (activityId: string) => void;
  // The id of the page's sentence about why this contact takes no changes,
  // while it does not. The upload hangs a file on the contact through the
  // contact's own write gate. The same fact refuses it as every other write
  // on the page.
  refusedReasonId?: string;
}>) {
  const t = useT();
  const [adding, setAdding] = useState(false);
  const [category, setCategory] = useState<Category | "">("");
  const { query, files, hasMore } = useContactFiles(contactId);
  const state = filesState(query, files.length, hasMore);
  const shown = category
    ? files.filter((file) => file.category === category)
    : files;

  return (
    <Panel
      title={t("tab.documents")}
      // Offered in every state. An empty library is what the upload exists to
      // leave. Withholding it on an empty or failed read would hide the
      // control when it is wanted.
      titleAction={
        <Button reasonId={refusedReasonId} onClick={() => setAdding(true)}>
          {t("docs.add.action")}
        </Button>
      }
      // Passed only where another page exists: the band draws itself whenever
      // it is given anything.
      footer={query.hasNextPage ? <LoadMoreButton query={query} /> : undefined}
    >
      <AddDocumentDialog
        anchor={{ record: "contact", id: contactId }}
        open={adding}
        onClose={() => setAdding(false)}
      />
      {files.length > 0 && (
        <PanelBody>
          <FileCuts
            files={files}
            complete={!hasMore}
            value={category}
            onChange={setCategory}
          />
        </PanelBody>
      )}
      {shown.length > 0 && (
        <FilesTable files={shown} onOpenEmail={onOpenEmail} />
      )}
      {state !== "ready" || shown.length === 0 ? (
        <PanelBody>
          <SurfaceState
            loadingLabel={t("tab.documents")}
            state={state === "ready" ? "empty" : state}
            emptyLabel={t(
              category ? "docs.noneInCategory" : "contact.documents.empty",
            )}
            detail={
              state === "failed"
                ? { onRetry: () => void query.refetch() }
                : undefined
            }
          >
            {null}
          </SurfaceState>
        </PanelBody>
      ) : null}
    </Panel>
  );
}

// The cuts through the files by category. A count only once the walk has
// read every page: before that it would be a floor printed as a count.
function FileCuts({
  files,
  complete,
  value,
  onChange,
}: Readonly<{
  files: readonly Attachment[];
  complete: boolean;
  value: Category | "";
  onChange: (next: Category | "") => void;
}>) {
  const t = useT();
  const countOf = (key: Category) =>
    files.filter((file) => file.category === key).length;
  const cuts = [...STEADY_CUTS, ...RARE_CUTS.filter((key) => countOf(key) > 0)];
  return (
    <FilterPills
      label={t("docs.filterLabel")}
      value={value}
      onChange={onChange}
      pills={[
        {
          value: "",
          label: t("docs.category.all"),
          count: complete ? files.length : undefined,
        },
        ...cuts.map((key) => ({
          value: key,
          label: t(CATEGORY_LABELS[key]),
          count: complete ? countOf(key) : undefined,
        })),
      ]}
    />
  );
}

// One row per file. The NAME is the download, as it is on the account's own
// list: the title if somebody gave it one, else the filename. The state shows
// only where a human or the source asserted one; nothing here infers it.
function FilesTable({
  files,
  onOpenEmail,
}: Readonly<{
  files: readonly Attachment[];
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const columns: DataTableColumn<Attachment>[] = [
    {
      key: "document",
      header: t("contact.documents.col.document"),
      grow: true,
      fold: "title",
      render: (file) => (
        <span className="contact-file">
          <a className="link-button" {...attachmentAnchor(file)}>
            {file.title || file.filename}
          </a>
          <FileOrigin origin={file.origin} onOpenEmail={onOpenEmail} />
        </span>
      ),
    },
    {
      key: "category",
      header: t("contact.documents.col.category"),
      render: (file) =>
        file.category ? t(CATEGORY_LABELS[file.category]) : null,
    },
    {
      key: "state",
      header: t("contact.documents.col.state"),
      render: (file) =>
        file.doc_state ? (
          <Badge tone={file.doc_state === "superseded" ? "warning" : "default"}>
            {t(STATE_LABELS[file.doc_state])}
          </Badge>
        ) : null,
    },
    {
      key: "added",
      header: t("contact.documents.col.added"),
      render: (file) => (
        <span className="t-num">
          {formatDateAbbrev(file.created_at, locale, recordZone)}
        </span>
      ),
    },
  ];
  return (
    <DataTable
      bleed
      fold
      label={t("tab.documents")}
      columns={columns}
      rows={[...files]}
      rowKey={(file) => file.id}
    />
  );
}
