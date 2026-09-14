import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch } from "../api/version";
import { useCanWriteRecord } from "../app/capability";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { Button, OverflowMenu } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { InlineText } from "../design-system/inlinechoice";
import { Panel, PanelBody } from "../design-system/panel";
import type { RecordPickerCandidate } from "../design-system/recordpicker";
import { SurfaceState } from "../design-system/surfacestate";
import { stable } from "../format/collate";
import { formatDayMonth } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import { AddEmploymentModal } from "./addemploymentmodal";
import { problemMessageOf, throwProblem } from "./common";
import {
  bodyState,
  type Contact360,
  useContactReadOnlyReason,
  withheldSections,
} from "./contactrail";
import { stillHeld, today } from "./employmentcurrency";

// --- Employers ---------------------------------------------------------

type Employment = components["schemas"]["Contact360Employment"];
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

// Contact360Employment is the 360's own projection of an employment edge — it
// carries `relationship_id` but not the relationship row's own `version`, and
// there is no `GET /relationships/{id}` in the contract to re-read one by id
// (relationships.tsx's RelationshipsTab keeps the same note, for the same
// reason). The one honest way to get an If-Match for a row this rail only
// knows by id is to re-read it through the list endpoint, scoped tight enough
// (this contact, this company, this kind) that it can only answer with the one
// edge this row is already showing.
async function fetchEmploymentVersion(
  employment: Employment,
  contactId: string,
): Promise<number | undefined> {
  const { data, error } = await api.GET("/relationships", {
    params: {
      query: {
        contact_id: contactId,
        company_id: employment.company_id,
        kind: "employment",
      },
    },
  });
  if (error) {
    throwProblem(error);
  }
  return data.data.find((rel) => rel.id === employment.relationship_id)
    ?.version;
}

// The one write path for everything on an employment row that is neither its
// creation nor its removal: the role InlineText commits below and the
// "mark as ended" verb both patch through here, so a role edit and an ended
// date answer the same version-skew and permission failures the same way.
//
// An unresolved version is refused here rather than sent unpinned: a write with
// no precondition writes straight over whatever changed underneath it instead of
// failing loud with a 409. The list scoping fetchEmploymentVersion uses can
// legitimately come back without this row (a narrower read scope, a paged
// response, an edge whose kind changed), and this rail is the one place that
// knows to say so — hence its own sentence for the reader rather than the shared
// refusal, which can only report that the write did not happen.
async function patchEmployment(
  employment: Employment,
  contactId: string,
  body: UpdateRelationshipRequest,
  t: ReturnType<typeof useT>,
): Promise<void> {
  const version = await fetchEmploymentVersion(employment, contactId);
  if (version === undefined) {
    throwProblem({
      detail: t("contact.rail.employmentVersionUnresolved"),
    });
  }
  const { error } = await api.PATCH("/relationships/{id}", {
    params: {
      path: { id: employment.relationship_id },
      ...ifMatch(version),
    },
    body,
  });
  if (error) {
    throwProblem(error);
  }
}

// The four writes the Companies section makes, sharing one invalidation:
// contact360 is what this section itself reads its rows from, and contactBrief
// comes with it because the brief's first sentence names the employer. The
// role InlineText below goes through `update` rather than calling
// patchEmployment on its own, so every write this section makes — role,
// ended date, create, remove — ends in the same refetch and the rail never
// shows a saved edit next to its own stale value.
function useEmploymentActions(contactId: string) {
  const t = useT();
  const queryClient = useQueryClient();
  const invalidate = async () => {
    await queryClient.invalidateQueries({
      queryKey: ["contact360", contactId],
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
  return { create, end, update, remove };
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
  const [adding, setAdding] = useState(false);
  const [removing, setRemoving] = useState<Employment | null>(null);
  const employments = [...(view.employments?.data ?? [])].sort(
    (a, b) =>
      Number(b.is_current_primary && stillHeld(b)) -
      Number(a.is_current_primary && stillHeld(a)),
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
          {employments.map((employment) => (
            <EmploymentRow
              key={employment.relationship_id}
              employment={employment}
              canEdit={canEdit}
              readOnlyReason={readOnlyReason}
              actions={actions}
              onRemove={() => setRemoving(employment)}
            />
          ))}
        </SurfaceState>
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

// One employment edge: the company it names, the role at that company (inline-
// editable — this is the ONE place a per-company title is corrected;
// `contact.title` is a different field, edited in Details above), the dates,
// and the row's own verbs folded behind an OverflowMenu — this row already
// carries a focusable inline-edit control, so the verbs stay out of the way
// until the row is hovered or that control (or the trigger itself) has
// focus, the same reveal the company page's task rows use for theirs.
function EmploymentRow({
  employment,
  canEdit,
  readOnlyReason,
  actions,
  onRemove,
}: Readonly<{
  employment: Employment;
  canEdit: boolean;
  readOnlyReason: string | undefined;
  actions: EmploymentActions;
  onRemove: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const detail = employmentDetail(employment, t, locale, recordZone);
  const ending =
    actions.end.isPending &&
    actions.end.variables?.relationship_id === employment.relationship_id;
  // isPending and isError can never both hold at once (one shared mutation
  // status behind both), so a failure that only rendered while "ending" was
  // also true could never actually draw: pending clears before error sets.
  // This row's own failure is instead keyed on the same identifier ending
  // uses, just checked against isError rather than isPending, so the row
  // that failed keeps its message once the mutation has settled.
  const endFailed =
    actions.end.isError &&
    actions.end.variables?.relationship_id === employment.relationship_id;
  return (
    <div className="pe-employment">
      <span className="pe-employment-body">
        <span className="pe-employment-company">
          {employment.company_name ? (
            <button
              type="button"
              className="pe-meta-link"
              onClick={() =>
                navigate({
                  screen: "companies",
                  id: employment.company_id,
                })
              }
            >
              {employment.company_name}
            </button>
          ) : (
            <span className="inlinetext">{t("field.unset")}</span>
          )}
          {employment.is_current_primary && stillHeld(employment) && (
            <span className="pe-rail-value-good">{t("rel.current")}</span>
          )}
        </span>
        <span className="pe-employment-role">
          <InlineText
            label={t("rel.role")}
            value={employment.role ?? ""}
            placeholder={t("field.addTitle")}
            canEdit={canEdit}
            readOnlyReason={readOnlyReason}
            onSave={(next) =>
              actions.update.mutateAsync({
                employment,
                body: { role: next || null },
              })
            }
          />
        </span>
        {detail && (
          <span className="pe-colleague-proof t-caption">{detail}</span>
        )}
      </span>
      {canEdit && (
        <span className="pe-employment-actions">
          <OverflowMenu label={t("record.moreActions")}>
            {!employment.ended_at && (
              <Button
                small
                disabled={ending}
                onClick={() => actions.end.mutate(employment)}
              >
                {t("contact.rail.markEnded")}
              </Button>
            )}
            <Button small variant="danger" onClick={onRemove}>
              {t("rel.remove")}
            </Button>
          </OverflowMenu>
        </span>
      )}
      {endFailed && (
        <p className="pe-colleague-proof t-caption" role="alert">
          {problemMessageOf(actions.end.error, t)}
        </p>
      )}
    </div>
  );
}
function employmentDetail(
  employment: Employment,
  t: ReturnType<typeof useT>,
  locale: Locale,
  recordZone: string,
): string {
  // The record's zone. These arrive as instants (`format: date-time`), but they
  // are WRITTEN from a date picker, so what is stored is midnight on the day a
  // human chose and the time carries no information. Rendered in a reader's own
  // zone west of UTC that midnight falls on the previous day, and two
  // colleagues would quote different start dates for one employment. The
  // record's zone is never behind UTC, so it renders the day that was picked.
  const start = employment.started_at
    ? formatDayMonth(employment.started_at, locale, recordZone)
    : undefined;
  const end = employment.ended_at
    ? formatDayMonth(employment.ended_at, locale, recordZone)
    : undefined;
  if (start && end) {
    return `${start} – ${end}`;
  }
  if (end) {
    return t("rel.endedOn", { when: end });
  }
  if (start) {
    return `${start} – ${t("rel.current")}`;
  }
  return "";
}
