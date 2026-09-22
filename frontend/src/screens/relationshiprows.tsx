// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The relationship edges of one record, as a panel and as the rows inside it.
//
// Split from relationships.tsx, which owns the scope vocabulary and the ADD
// form, because the rows grew a second shape: the deal draws its own panel
// around them and the file was at its length ceiling. The import runs one way,
// rows → relationships, so the add verb and the row verbs still agree on what a
// scope is.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useId, useState } from "react";
import { api } from "../api/client";
import { ifMatch, requireVersion } from "../api/version";
import { useCanWrite } from "../app/capability";
import { Badge, Button, EmptyState, Modal } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { type Fact, FactList } from "../design-system/factlist";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { useT } from "../i18n";
import { problemMessageOf, QueryGate, throwProblem } from "./common";
import { EditAction } from "./edit";
import { EntityRef } from "./entityref";
import { KIND_LABELS } from "./relationshipkinds";
import {
  AddRelationshipAction,
  counterpartyRef,
  dateRange,
  fetchRelationships,
  invalidateAfterEdge,
  orNull,
  type Relationship,
  type RelationshipScope,
  relationshipEditFields,
  scopeCopy,
  scopeQueryKey,
} from "./relationships";

/**
 * RelationshipsTab is the edges of one record, as a panel that names itself.
 *
 * The panel and the rows are separable because the deal draws its own panel:
 * its committee card puts the coverage map above these rows and an engagement
 * cell beside them (deal360/dealcommitteecard.tsx), and there is exactly one writer
 * for create, edit and remove whichever panel the rows stand in.
 */
export function RelationshipsTab({
  scope,
  refusedReasonId,
}: Readonly<{
  scope: RelationshipScope;
  // See AddRelationshipAction: the anchor page's one read-only sentence,
  // which every write here points at when the anchor refuses changes.
  refusedReasonId?: string;
}>) {
  const t = useT();
  const copy = scopeCopy(scope);
  const canCreate = useCanWrite("relationship", "create");
  return (
    <Panel
      title={t(copy.title)}
      titleAction={
        canCreate ? (
          <AddRelationshipAction
            scope={scope}
            refusedReasonId={refusedReasonId}
          />
        ) : undefined
      }
    >
      <PanelBody>
        <RelationshipRows scope={scope} refusedReasonId={refusedReasonId} />
      </PanelBody>
    </Panel>
  );
}

/**
 * RelationshipRows is the edges themselves, with no panel around them: the
 * read, the row verbs and the two-step remove.
 *
 * `extra` is one more label→value the generic edge cannot know, drawn after the
 * counterparty: the deal knows which stakeholders have answered. It rides these
 * rows rather than standing as a second list beside them, because a reader
 * comparing "who is on this" against "who has replied" across two lists is
 * doing the join the page should have done.
 *
 * `stacked` is the same rows in the other shape of box, the way
 * `EmailReference` takes its own: label left and value right, each edge under
 * the last, for a narrow column. A four-column table in a half-width panel ran
 * its verbs off the right edge — the controls were there and unreachable. This
 * is a prop rather than a second component because a second one would be a
 * second writer the day either grew a verb.
 */
export function RelationshipRows({
  scope,
  refusedReasonId,
  extra,
  stacked,
}: Readonly<{
  scope: RelationshipScope;
  refusedReasonId?: string;
  extra?: {
    label: string;
    render: (rel: Relationship) => ReactNode;
  };
  stacked?: boolean;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const headingId = useId();
  const copy = scopeCopy(scope);
  // The object half of each verb's gate, asked as the server asks it
  // (relationship:create on an add, :update on an edit, :delete on a
  // removal). A verb the role holds no grant for is withheld outright — there
  // is no fact about the record to report — where the anchor's refusal above
  // keeps the verb and says why.
  const canUpdate = useCanWrite("relationship", "update");
  const canDelete = useCanWrite("relationship", "delete");
  const query = useQuery({
    queryKey: scopeQueryKey(scope),
    queryFn: () => fetchRelationships(scope),
  });

  // Two-step confirm, mirroring ArchiveAction (archive.tsx) — Remove is a
  // hard DELETE with no restore path, so it never fires from a single click.
  // The ROW, not its id: what the write invalidates depends on whether the edge
  // names a deal, and an id alone cannot answer that.
  const [removing, setRemoving] = useState<Relationship | null>(null);

  const remove = useMutation({
    mutationFn: async (doomed: Relationship) => {
      const { data, error } = await api.DELETE("/relationships/{id}", {
        params: { path: { id: doomed.id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (_removed, doomed) => {
      invalidateAfterEdge(queryClient, doomed.deal_id != null);
      setRemoving(null);
    },
  });

  // The same facts the table draws as columns, as label→value pairs for the
  // stacked shape. Built from one list so a column added to the table is a row
  // here rather than a fact one shape quietly stopped showing. An absent value
  // still prints its row: a stakeholder with no role recorded is a fact about
  // the deal, and dropping the row would read as a contact with no role to give.
  const factsFor = (rel: Relationship): Fact[] => {
    const ref = counterpartyRef(rel, scope);
    return [
      ...(copy.singleKind
        ? []
        : [
            {
              key: "kind",
              term: t("rel.kind"),
              value: <Badge>{t(KIND_LABELS[rel.kind])}</Badge>,
            },
          ]),
      { key: "role", term: t("rel.role"), value: rel.role ?? "—" },
      {
        key: "counterparty",
        term: t("rel.counterparty"),
        value: ref ? <EntityRef kind={ref.kind} id={ref.id} /> : "—",
      },
      ...(extra
        ? [{ key: "extra", term: extra.label, value: extra.render(rel) }]
        : []),
      { key: "dates", term: t("rel.dates"), value: dateRange(rel, t) },
    ];
  };

  // ONE spelling of this edge's verbs, for both shapes of row: the table's
  // trailing cell and the stacked block's foot. Two copies would be two
  // versions of a hard DELETE the day either one is touched.
  const verbsFor = (rel: Relationship) => (
    <div
      style={{
        display: "flex",
        gap: "var(--space-2)",
        // Stacked, the verbs sit under the facts rather than in a cell of their
        // own, so they go to the far end: the labels hold the left edge and a
        // control on that same edge reads as one more of them.
        justifyContent: stacked ? "flex-end" : undefined,
      }}
    >
      {canUpdate && (
        <EditAction
          disabledReasonId={refusedReasonId}
          label={t("record.edit")}
          savedMessage={t("rel.saveDone")}
          fields={relationshipEditFields}
          record={{
            id: rel.id,
            version: rel.version,
            role: rel.role ?? "",
            started_at: rel.started_at ?? "",
            ended_at: rel.ended_at ?? "",
          }}
          update={async (values, _rows, opened) => {
            const { data, error } = await api.PATCH("/relationships/{id}", {
              params: {
                path: { id: rel.id },
                ...ifMatch(requireVersion(opened?.version)),
              },
              body: {
                role: orNull(values.role),
                started_at: orNull(values.started_at),
                ended_at: orNull(values.ended_at),
              },
            });
            if (error) {
              throwProblem(error);
            }
            return data;
          }}
          invalidate="relationships"
          recordKey="relationship"
        />
      )}
      {canDelete && (
        <Button
          variant="danger"
          reasonId={refusedReasonId}
          onClick={() => setRemoving(rel)}
          data-testid="remove-relationship"
        >
          {t("rel.remove")}
        </Button>
      )}
    </div>
  );

  // The table's columns, named once outside the render callback: built inline,
  // the callback carried every column's own branching as well as the choice of
  // shape, and read as one function doing two jobs.
  const columns = [
    ...(copy.singleKind
      ? []
      : [
          {
            key: "kind",
            header: t("rel.kind"),
            render: (rel: Relationship) => (
              <Badge>{t(KIND_LABELS[rel.kind])}</Badge>
            ),
          },
        ]),
    {
      key: "role",
      header: t("rel.role"),
      render: (rel: Relationship) => rel.role ?? "",
    },
    {
      key: "counterparty",
      header: t("rel.counterparty"),
      render: (rel: Relationship) => {
        const ref = counterpartyRef(rel, scope);
        return ref ? <EntityRef kind={ref.kind} id={ref.id} /> : <span>—</span>;
      },
    },
    ...(extra
      ? [
          {
            key: "extra",
            header: extra.label,
            render: extra.render,
          },
        ]
      : []),
    {
      key: "dates",
      header: t("rel.dates"),
      render: (rel: Relationship) => dateRange(rel, t),
    },
    {
      key: "actions",
      header: "",
      render: verbsFor,
    },
  ];

  return (
    <>
      <QueryGate query={query} pendingLabel={t(copy.title)}>
        {(rows) => {
          if (rows.length === 0) {
            // Stacked rows are panel-level children, so the sentence that
            // stands where they would stand needs the body's padding that they
            // deliberately escape.
            return stacked ? (
              <PanelBody>
                <EmptyState>{t(copy.empty)}</EmptyState>
              </PanelBody>
            ) : (
              <EmptyState>{t(copy.empty)}</EmptyState>
            );
          }
          if (stacked) {
            return rows.map((rel) => (
              <PanelRow key={rel.id}>
                <FactList facts={factsFor(rel)} />
                {verbsFor(rel)}
              </PanelRow>
            ));
          }
          return (
            <DataTable
              label={t(copy.title)}
              columns={columns}
              rows={rows}
              rowKey={(rel) => rel.id}
            />
          );
        }}
      </QueryGate>
      <Modal
        open={removing !== null}
        onClose={() => {
          setRemoving(null);
          remove.reset();
        }}
        labelledBy={headingId}
      >
        <Heading
          size="large"
          id={headingId}
          className="t-h2"
          style={{ marginBottom: "var(--space-3)" }}
        >
          {t("rel.remove")}
        </Heading>
        <p style={{ marginBottom: "var(--space-4)" }}>
          {t("rel.removeConfirm")}
        </p>
        {remove.isError && (
          <p style={{ color: "var(--dangerText)" }}>
            {problemMessageOf(remove.error, t)}
          </p>
        )}
        <div
          style={{
            display: "flex",
            gap: "var(--gapActions)",
            justifyContent: "flex-end",
          }}
        >
          <Button
            // The mutation is reset with the dialog, not just the row it was
            // aimed at: a failed remove left its sentence behind, and the next
            // seat's dialog opened carrying an error for a write nobody had
            // attempted on it.
            onClick={() => {
              setRemoving(null);
              remove.reset();
            }}
            disabled={remove.isPending}
          >
            {t("create.cancel")}
          </Button>
          <Button
            variant="danger"
            onClick={() => {
              if (removing) {
                remove.mutate(removing);
              }
            }}
            disabled={remove.isPending}
            data-testid="remove-relationship-confirm"
          >
            {t("rel.remove")}
          </Button>
        </div>
      </Modal>
    </>
  );
}
