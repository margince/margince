// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The team's lists: every Live List and Shortlist this reader may find, with
// what each is for, how many of its members they can see, who looks after it,
// and whether it needs someone. A row opens the list.

import { useState } from "react";
import { navigate } from "../app/router";
import {
  Badge,
  Button,
  Field,
  SearchField,
  SegmentedControl,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DataTable } from "../design-system/datatable";
import { Panel, PanelBody } from "../design-system/panel";
import { Select } from "../design-system/select";
import { SurfaceState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf } from "./common";
import {
  type List,
  type ListRecordType,
  useCreateList,
  useLists,
} from "./lists.queries";

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

const KIND_FILTERS = ["all", "dynamic", "static"] as const;
type KindFilter = (typeof KIND_FILTERS)[number];

/** What a list is, in the reader's words rather than the wire's. */
export function ListKindBadge({
  list,
}: Readonly<{ list: Pick<List, "list_type"> }>) {
  const t = useT();
  return list.list_type === "dynamic" ? (
    <Badge tone="accent">{t("lists.kind.live")}</Badge>
  ) : (
    <Badge tone="info">{t("lists.kind.shortlist")}</Badge>
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
  return null;
}

export function ListLibrary() {
  const t = useT();
  const { locale } = useLocale();
  const [query, setQuery] = useState("");
  const [kind, setKind] = useState<KindFilter>("all");
  const lists = useLists({
    q: query,
    listType: kind === "all" ? undefined : kind,
  });
  const rows = lists.data?.data ?? [];

  return (
    <Panel title={t("lists.library.title")} actions={<NewShortlistAction />}>
      <PanelBody className="lists-library-dials">
        <SearchField
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder={t("lists.library.search")}
          aria-label={t("lists.library.search")}
        />
        <SegmentedControl
          options={KIND_FILTERS}
          value={kind}
          onChange={setKind}
          labels={{
            all: t("lists.library.all"),
            dynamic: t("lists.kind.live"),
            static: t("lists.kind.shortlist"),
          }}
          label={t("lists.library.kind")}
        />
      </PanelBody>
      <PanelBody>
        <SurfaceState
          state={
            lists.isPending
              ? "loading"
              : lists.isError
                ? "unavailable"
                : rows.length > 0
                  ? "ready"
                  : "empty"
          }
          emptyLabel={t("lists.library.empty")}
          loadingLabel={t("lists.library.loading")}
          loadingLines={4}
        >
          <DataTable
            label={t("lists.library.title")}
            rows={rows}
            rowKey={(list) => list.id}
            onRowClick={(list) => navigate({ screen: "lists", id: list.id })}
            columns={[
              {
                key: "name",
                header: t("lists.col.name"),
                grow: true,
                render: (list) => (
                  <span className="lists-library-name">
                    <strong>{list.name}</strong>
                    {list.purpose && (
                      <span className="t-caption">{list.purpose}</span>
                    )}
                  </span>
                ),
              },
              {
                key: "kind",
                header: t("lists.col.kind"),
                render: (list) => <ListKindBadge list={list} />,
              },
              {
                key: "type",
                header: t("lists.col.recordType"),
                render: (list) => t(RECORD_TYPE_LABEL[list.entity_type]),
              },
              {
                key: "count",
                header: t("lists.col.count"),
                align: "end",
                render: (list) =>
                  list.visible_count == null
                    ? "—"
                    : formatNumber(list.visible_count, locale),
              },
              {
                key: "steward",
                header: t("lists.col.steward"),
                render: (list) => (
                  <span className="lists-library-steward">
                    {list.steward_name ?? t("lists.noSteward")}
                    <ListHealthBadge list={list} />
                  </span>
                ),
              },
              {
                key: "sharing",
                header: t("lists.col.sharing"),
                render: (list) => t(SHARING_LABEL[list.sharing]),
              },
            ]}
          />
        </SurfaceState>
      </PanelBody>
    </Panel>
  );
}

export const SHARING_LABEL: Record<List["sharing"], MessageKey> = {
  private: "lists.sharing.private",
  team: "lists.sharing.team",
  workspace: "lists.sharing.workspace",
};

/**
 * "New Shortlist": an empty list of chosen records, named and typed here and
 * filled from a selection or a record page. A Live List is made from the
 * builder instead, where its filter is previewed before it is saved.
 */
function NewShortlistAction() {
  const t = useT();
  const create = useCreateList();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [purpose, setPurpose] = useState("");
  const [entityType, setEntityType] = useState<ListRecordType>("contact");
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
      </ConfirmModal>
    </>
  );
}
