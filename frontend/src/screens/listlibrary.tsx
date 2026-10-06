// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The library's rows and the words a list is described by: its kind, its
// health, what it gained since the last visit, and how many records it holds
// that this reader can see. Every saved view and list shares one table, so a
// view and a list read alike wherever they sit.

import { type ReactNode, useState } from "react";
import { navigate, routeHash } from "../app/router";
import {
  Badge,
  Button,
  Field,
  OverflowMenu,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { Select } from "../design-system/select";
import { useTruncationTooltip } from "../design-system/tooltip";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf } from "./common";
import { EditFilterAction, mayEditFilter } from "./filterlistedit";
import { RECORDS_COUNT_LABEL } from "./filtersaddress";
import {
  isArchived,
  keyOf,
  type LibraryGroup,
  type LibraryItem,
  nameOf,
  resourceOf,
} from "./library";
import "./library.css";
import { type List, type ListRecordType, useCreateList } from "./lists.queries";
import {
  type ListAudience,
  ListAudienceFields,
  useListAudienceLabel,
} from "./listsharing";
import { DeleteViewAction, RenameViewAction } from "./viewactions";

/** The record types a list is made for on this screen, and their words. */
export const LIST_RECORD_TYPES = [
  "contact",
  "company",
  "deal",
  "lead",
] as const satisfies readonly ListRecordType[];

export const RECORD_TYPE_LABEL: Record<ListRecordType, MessageKey> = {
  contact: "lists.type.contact",
  company: "lists.type.company",
  deal: "lists.type.deal",
  lead: "lists.type.lead",
  project: "lists.type.project",
};

/**
 * What a list is, in the reader's words rather than the wire's. Neutral: a
 * kind is a fact, and the accent belongs to the page's one primary action.
 */
export function ListKindBadge({
  list,
}: Readonly<{ list: Pick<List, "list_type"> }>) {
  const t = useT();
  return (
    <Badge>
      {t(
        list.list_type === "dynamic"
          ? "lists.kind.live"
          : "lists.kind.shortlist",
      )}
    </Badge>
  );
}

/** A list that needs somebody, said on the row; a healthy one says nothing. */
export function ListHealthBadge({
  list,
}: Readonly<{ list: Pick<List, "health"> }>) {
  const t = useT();
  if (list.health === "ownerless") {
    return <Badge tone="warning">{t("lists.health.ownerless")}</Badge>;
  }
  if (list.health === "invalid") {
    return <Badge tone="danger">{t("lists.health.invalid")}</Badge>;
  }
  if (list.health === "retired_field") {
    return <Badge tone="warning">{t("lists.health.retiredField")}</Badge>;
  }
  return null;
}

/**
 * Which table a group draws. Only me and Shared carry the kind, since both
 * hold views and lists; Shared adds who can find each row. With lists off,
 * every row is a saved view, so the kind says nothing and is left out.
 */
export type LibraryTableGroup = LibraryGroup | "views";

/**
 * The library's rows: one saved view or list each. The name is a real link,
 * stretched over its row, so a row opens from the keyboard and in a new tab,
 * and a press on ⋯ or inside a dialog it opened never reaches a row handler.
 * Folded to a phone's width, the facts move under the name.
 */
export function LibraryTable({
  label,
  items,
  group,
  captionOf,
  folded,
}: Readonly<{
  label: string;
  items: readonly LibraryItem[];
  group: LibraryTableGroup;
  captionOf: (item: LibraryItem) => string;
  folded: boolean;
}>) {
  const t = useT();
  const audienceOf = useListAudienceLabel();
  const name: DataTableColumn<LibraryItem> = {
    key: "name",
    header: t("lists.col.name"),
    grow: true,
    render: (item) => (
      <LibraryName
        item={item}
        caption={captionOf(item)}
        facts={folded ? <LibraryFacts item={item} group={group} /> : null}
      />
    ),
  };
  const more: DataTableColumn<LibraryItem> = {
    key: "more",
    header: "",
    render: (item) => <LibraryRowMenu item={item} />,
  };
  const kind: DataTableColumn<LibraryItem> = {
    key: "kind",
    header: t("lists.col.kind"),
    render: (item) => <LibraryKind item={item} />,
  };
  const records: DataTableColumn<LibraryItem> = {
    key: "records",
    header: t("lists.col.recordType"),
    align: "end",
    render: (item) => <LibraryRecords item={item} />,
  };
  const who: DataTableColumn<LibraryItem> = {
    key: "who",
    header: t("lists.sharingLabel"),
    render: (item) => (item.kind === "list" ? audienceOf(item.list) : null),
  };
  const columns = folded
    ? [name, more]
    : [
        name,
        ...(group === "views" ? [] : [kind]),
        records,
        ...(group === "shared" ? [who] : []),
        more,
      ];
  return (
    <div className="library-table">
      <DataTable
        label={label}
        rows={[...items]}
        rowKey={keyOf}
        columns={columns}
      />
    </div>
  );
}

function hrefOf(item: LibraryItem): string {
  return item.kind === "view"
    ? routeHash({ screen: "filters", id: item.tab, id2: item.view.id })
    : routeHash({ screen: "lists", id: item.list.id });
}

function LibraryName({
  item,
  caption,
  facts,
}: Readonly<{ item: LibraryItem; caption: string; facts: ReactNode }>) {
  const t = useT();
  const tip = useTruncationTooltip<HTMLSpanElement>(caption);
  return (
    <span className="library-name-cell">
      <span className="library-name-line">
        <a className="library-name" href={hrefOf(item)}>
          {nameOf(item)}
        </a>
        {item.kind === "list" && <ListHealthBadge list={item.list} />}
        {isArchived(item) && <Badge>{t("record.archived")}</Badge>}
      </span>
      {caption && (
        <span
          className="t-caption library-caption"
          ref={tip.ref}
          {...tip.trigger}
        >
          {caption}
          {tip.tip}
        </span>
      )}
      {facts}
    </span>
  );
}

/** A folded row's third line: what the hidden columns would have said. */
function LibraryFacts({
  item,
  group,
}: Readonly<{ item: LibraryItem; group: LibraryTableGroup }>) {
  const audienceOf = useListAudienceLabel();
  return (
    <span className="t-caption library-facts">
      {group !== "views" && <LibraryKind item={item} />}
      <LibraryRecords item={item} />
      {group === "shared" && item.kind === "list" && (
        <span>{audienceOf(item.list)}</span>
      )}
    </span>
  );
}

function LibraryKind({ item }: Readonly<{ item: LibraryItem }>) {
  const t = useT();
  return item.kind === "view" ? (
    <Badge>{t("filters.library.kindView")}</Badge>
  ) : (
    <ListKindBadge list={item.list} />
  );
}

/**
 * How many records a list holds that this reader can see, in its noun. A
 * saved view counts nothing until it is opened, and a list the server could
 * not count says only its type: never a dash and never a zero.
 */
function LibraryRecords({ item }: Readonly<{ item: LibraryItem }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const type = resourceOf(item);
  if (item.kind === "view" || item.list.visible_count == null) {
    return <span>{t(RECORD_TYPE_LABEL[type])}</span>;
  }
  const count = item.list.visible_count;
  const records = plural(RECORDS_COUNT_LABEL[type], count, {
    count: formatNumber(count, locale),
  });
  return (
    <span className="library-records">
      <span aria-hidden="true">{records}</span>
      <span className="sr-only">
        {t("filters.library.recordsSeen", { records })}
      </span>
      {item.list.list_type === "dynamic" && <ListPulseBadge list={item.list} />}
    </span>
  );
}

/**
 * A row's own verbs: a saved view is renamed or deleted here; a Live List's
 * filter is edited by whoever may change it. A Shortlist has none.
 */
function LibraryRowMenu({ item }: Readonly<{ item: LibraryItem }>) {
  const t = useT();
  const label = t("filters.library.rowMore", { name: nameOf(item) });
  if (item.kind === "view") {
    return (
      <span className="library-more">
        <OverflowMenu label={label}>
          <RenameViewAction view={item.view} />
          <DeleteViewAction view={item.view} />
        </OverflowMenu>
      </span>
    );
  }
  if (!mayEditFilter(item.list)) {
    return null;
  }
  return (
    <span className="library-more">
      <OverflowMenu label={label}>
        <EditFilterAction list={item.list} />
      </OverflowMenu>
    </span>
  );
}

/**
 * What a Live List gained and lost since the reader last opened it, counting
 * only records they can see. Nothing on a first visit or a quiet list.
 */
export function ListPulseBadge({
  list,
}: Readonly<{ list: Pick<List, "since_last_visit"> }>) {
  const t = useT();
  const pulse = list.since_last_visit;
  if (!pulse || pulse.entered + pulse.left === 0) {
    return null;
  }
  const counts = { entered: String(pulse.entered), left: String(pulse.left) };
  return (
    <Badge>
      <span aria-hidden="true">{t("lists.pulse.chip", counts)}</span>
      <span className="sr-only">{t("lists.pulse.label", counts)}</span>
    </Badge>
  );
}

/**
 * "New Shortlist": an empty list of chosen records, named and typed here and
 * filled from a selection or a record page. A Live List is made from the
 * builder instead, where its filter is previewed before it is saved. It
 * starts with the audience its caller names.
 */
export function NewShortlistAction({
  defaultAudience,
}: Readonly<{ defaultAudience: ListAudience }>) {
  const t = useT();
  const create = useCreateList();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [purpose, setPurpose] = useState("");
  const [entityType, setEntityType] = useState<ListRecordType>("contact");
  const [audience, setAudience] = useState<ListAudience>(defaultAudience);
  return (
    <>
      <Button onClick={() => setOpen(true)}>{t("lists.newShortlist")}</Button>
      <ConfirmModal
        open={open}
        onClose={() => setOpen(false)}
        title={t("lists.newShortlistTitle")}
        confirmLabel={t("lists.create")}
        confirmDisabled={name.trim() === ""}
        pending={create.isPending}
        error={create.isError ? problemMessageOf(create.error, t) : null}
        onConfirm={() =>
          create.mutate(
            {
              name: name.trim(),
              purpose: purpose.trim(),
              entityType,
              listType: "static",
              sharing: audience.sharing,
              teamId: audience.sharing === "team" ? audience.teamId : null,
            },
            {
              onSuccess: (list) => {
                setOpen(false);
                navigate({ screen: "lists", id: list.id });
              },
            },
          )
        }
      >
        <Field label={t("lists.name")}>
          {(control) => (
            <TextInput
              {...control}
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          )}
        </Field>
        <Field label={t("lists.recordTypeLabel")}>
          {(control) => (
            <Select
              {...control}
              value={entityType}
              onChange={(next) => setEntityType(next as ListRecordType)}
              options={LIST_RECORD_TYPES.map((type) => ({
                value: type,
                label: t(RECORD_TYPE_LABEL[type]),
              }))}
            />
          )}
        </Field>
        <Field label={t("lists.purpose")}>
          {(control) => (
            <Textarea
              {...control}
              value={purpose}
              onChange={(event) => setPurpose(event.target.value)}
            />
          )}
        </Field>
        <ListAudienceFields
          value={audience}
          onChange={setAudience}
          ownerIsReader
        />
      </ConfirmModal>
    </>
  );
}
