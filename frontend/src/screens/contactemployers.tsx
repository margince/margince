import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWriteRecord } from "../app/capability";
import { Button } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Panel, PanelBody } from "../design-system/panel";
import type { RecordPickerCandidate } from "../design-system/recordpicker";
import { SurfaceState } from "../design-system/surfacestate";
import { stable } from "../format/collate";
import { useT } from "../i18n";
import { AddEmploymentModal } from "./addemploymentmodal";
import { problemMessageOf, throwProblem } from "./common";
import { EmploymentRow } from "./contactemploymentrow";
import {
  bodyState,
  type Contact360,
  useContactReadOnlyReason,
  withheldSections,
} from "./contactrail";
import { stillHeld, today } from "./employmentcurrency";
import { EmploymentEdit } from "./employmentedit";
import { ImportedEmploymentHistory } from "./employmentimport";
import { useEmploymentPages } from "./employmentpages";
import { patchEmployment } from "./employmentpatch";

// --- Employers ---------------------------------------------------------

export type Employment = components["schemas"]["Contact360Employment"];
type CreateRelationshipRequest =
  components["schemas"]["CreateRelationshipRequest"];
type UpdateRelationshipRequest =
  components["schemas"]["UpdateRelationshipRequest"];

export async function searchCompanyCandidates(
  q: string,
): Promise<RecordPickerCandidate[]> {
  const { data, error } = await api.GET("/companies", {
    params: { query: { q, limit: 10 } },
  });
  if (error) {
    throwProblem(error);
  }
  return data.data.map((company) => ({
    id: company.id,
    name: company.display_name,
  }));
}

// Refresh the record, paginated roles and brief together after any employment
// edit so the saved value and employer summary agree.
function useEmploymentActions(contactId: string) {
  const t = useT();
  const queryClient = useQueryClient();
  const invalidate = async () => {
    await queryClient.invalidateQueries({
      queryKey: ["contact360", contactId],
    });
    await queryClient.invalidateQueries({
      queryKey: ["contactEmployments", contactId],
    });
    await queryClient.invalidateQueries({
      queryKey: ["contactBrief", contactId],
    });
  };
  const create = useMutation({
    mutationFn: async (body: CreateRelationshipRequest) => {
      const { data, error } = await api.POST("/relationships", { body });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: invalidate,
  });
  const end = useMutation({
    mutationFn: (employment: Employment) =>
      patchEmployment(employment, contactId, { ended_at: today() }, t),
    onSuccess: invalidate,
  });
  const update = useMutation({
    mutationFn: ({
      employment,
      body,
    }: {
      employment: Employment;
      body: UpdateRelationshipRequest;
    }) => patchEmployment(employment, contactId, body, t),
    onSuccess: invalidate,
  });
  const remove = useMutation({
    mutationFn: async (relationshipId: string) => {
      const { error } = await api.DELETE("/relationships/{id}", {
        params: { path: { id: relationshipId } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: invalidate,
  });
  return { create, end, update, remove, invalidate };
}

export type EmploymentActions = ReturnType<typeof useEmploymentActions>;

// A contact can hold more than one employment edge at once, so this is a
// list rather than the single Details row it used to be. The current
// employer leads and carries an explicit marker: `is_current_primary` is
// the recorded fact, not something a reader should have to derive from
// whether `ended_at` happens to be blank — a rep who has to check dates to
// know which company to email has already lost the point of the marker.
export function Employers({ view }: Readonly<{ view: Contact360 }>) {
  const t = useT();
  const contact = view.contact;
  const readOnlyReason = useContactReadOnlyReason(contact);
  // The same decision and the same read-only reasons DetailsGrid gates its own
  // edit affordances on — a reader who cannot edit the contact's own fields
  // cannot edit which company they work at either.
  const canEdit = useCanWriteRecord("contact", contact) && !readOnlyReason;
  const actions = useEmploymentActions(contact.id);
  const more = useEmploymentPages(view);
  const allEmployments = [
    ...new Map(
      [
        ...(more.data?.pages.flatMap((page) => page.roles) ?? []),
        ...(view.employments?.data ?? []),
      ].map((role) => [role.relationship_id, role]),
    ).values(),
  ];
  const [adding, setAdding] = useState(false);
  // The row being edited, kept after the dialog closes so it still has an
  // employment to draw while it animates out. `editingOpen` is what is open,
  // and `seq` rides in the key below so every press opens a dialog seeded
  // afresh from the row — the reset the old unmount gave for free.
  const [editing, setEditing] = useState<Readonly<{
    row: Employment;
    seq: number;
  }> | null>(null);
  const [editingOpen, setEditingOpen] = useState(false);
  const [removing, setRemoving] = useState<Employment | null>(null);
  const primaryCompany = allEmployments.find(
    (e) => e.is_current_primary && stillHeld(e),
  )?.company_id;
  const employments = [...allEmployments].sort(
    (a, b) =>
      Number(b.company_id === primaryCompany) -
        Number(a.company_id === primaryCompany) ||
      stable(a.company_id, b.company_id) ||
      stable(b.started_at ?? "", a.started_at ?? ""),
  );
  // Every company this contact already has a live edge to — the 360 projection
  // drops an edge the moment it is removed, so this list IS the live set,
  // nothing further to filter. AddEmploymentModal excludes these from its
  // own picker so a rep cannot draw a second edge to a company already on
  // this list.
  //
  // Memoized on a primitive key rather than on `employments` itself: the
  // array here is rebuilt fresh every render (the sort above always returns
  // a new one), and AddEmploymentModal's own searchTargets treats a new
  // identity as a new search space and clears the picker's candidates —
  // so the set only gets a new identity when the set of ids it names
  // actually changes.
  // `stable` rather than the reader's collation, because this string is only
  // ever compared against a previous rendering of itself: whose locale produced
  // it must not be part of the answer.
  const connectedCompanyKey = employments
    .map((employment) => employment.company_id)
    .sort(stable)
    .join(",");
  const connectedCompanyIds = useMemo(
    () => (connectedCompanyKey === "" ? [] : connectedCompanyKey.split(",")),
    [connectedCompanyKey],
  );
  return (
    <Panel
      title={t("contact.rail.employmentTitle")}
      titleAction={
        canEdit ? (
          <Button small variant="ghost" onClick={() => setAdding(true)}>
            {t("contact.rail.addEmployment")}
          </Button>
        ) : undefined
      }
    >
      <PanelBody>
        <SurfaceState
          loadingLabel={t("contact.rail.employmentTitle")}
          state={bodyState(
            withheldSections(view).employments,
            employments.length,
          )}
          emptyLabel={t("contact.rail.noEmployment")}
        >
          {employments.map((employment, index) => (
            <EmploymentRow
              key={employment.relationship_id}
              showCompany={
                index === 0 ||
                employments[index - 1]?.company_id !== employment.company_id
              }
              employment={employment}
              // The title the contact's own record carries stands in for a
              // role not yet written on the current employment: the same fact
              // the header shows under the name, offered here where a reader
              // can confirm it as the role at THIS company. `stillHeld`
              // rather than the raw `is_current_primary` flag, so this
              // matches exactly the row the "current" badge below marks:
              // a former job whose flag was never cleared gets neither, and
              // only the primary one: the title is contact-wide, and offered
              // on every live row it would be saved to each of them.
              fallbackRole={
                stillHeld(employment) && employment.is_current_primary
                  ? (contact.title ?? undefined)
                  : undefined
              }
              canEdit={canEdit}
              readOnlyReason={readOnlyReason}
              actions={actions}
              onRemove={() => setRemoving(employment)}
              onEdit={() => {
                setEditing((prior) => ({
                  row: employment,
                  seq: (prior?.seq ?? 0) + 1,
                }));
                setEditingOpen(true);
              }}
            />
          ))}
        </SurfaceState>
        {more.isError && <p role="alert">{problemMessageOf(more.error, t)}</p>}
        {more.hasNextPage && (
          <Button
            small
            pending={more.isFetchingNextPage}
            onClick={() => more.fetchNextPage()}
          >
            {t("employment.more")}
          </Button>
        )}
        {!withheldSections(view).employments && (
          <ImportedEmploymentHistory
            key={contact.id}
            view={{
              ...view,
              employments: { data: employments, page: { has_more: false } },
            }}
            canEdit={canEdit}
          />
        )}
        {/* The guard falls only before the first row is ever edited: `editing`
            outlives the close, so from then on the dialog stays mounted and
            leaves with the employment it was opened on still drawn. */}
        {editing !== null && (
          <EmploymentEdit
            key={`${contact.id}:${editing.row.relationship_id}:${editing.seq}`}
            employment={editing.row}
            open={editingOpen}
            contactId={contact.id}
            onClose={() => setEditingOpen(false)}
            onSaved={actions.invalidate}
          />
        )}
        <AddEmploymentModal
          open={adding}
          onClose={() => setAdding(false)}
          contactId={contact.id}
          create={actions.create}
          excludedCompanyIds={connectedCompanyIds}
          hasCurrentEmployment={employments.some(stillHeld)}
        />
        {/* Remove is the irreversible verb — the connection and its history are
          gone, not merely dated — so it is the one that sits behind a
          confirm, unlike "mark as ended" which is an ordinary field edit. */}
        <ConfirmModal
          open={removing !== null}
          onClose={() => {
            setRemoving(null);
            actions.remove.reset();
          }}
          title={t("contact.rail.removeEmploymentTitle")}
          confirmLabel={t("rel.remove")}
          confirmVariant="danger"
          onConfirm={() => {
            if (removing) {
              actions.remove.mutate(removing.relationship_id, {
                onSuccess: () => setRemoving(null),
              });
            }
          }}
          pending={actions.remove.isPending}
          error={
            actions.remove.isError
              ? problemMessageOf(actions.remove.error, t)
              : null
          }
        >
          <p className="t-body">
            {t("contact.rail.removeEmploymentBody", {
              company: removing?.company_name ?? t("field.unset"),
            })}
          </p>
        </ConfirmModal>
      </PanelBody>
    </Panel>
  );
}
