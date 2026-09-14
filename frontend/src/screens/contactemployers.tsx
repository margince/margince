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
import { formatDateAbbrev } from "../format/format";
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
import { EmploymentEdit } from "./employmentedit";
import { ImportedEmploymentHistory } from "./employmentimport";
import { useEmploymentPages } from "./employmentpages";
import { sameEditValue, saveIndependentEdit } from "./independentedit";

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

// Re-read through the scoped list endpoint when a concurrent edit requires a
// comparison; the relationship contract has no single-record GET.
export async function patchEmployment(
  employment: Employment,
  contactId: string,
  body: UpdateRelationshipRequest,
  t: ReturnType<typeof useT>,
): Promise<void> {
  const read = async () => {
    const { data, error } = await api.GET("/relationships", {
      params: {
        query: {
          contact_id: contactId,
          company_id: employment.company_id,
          kind: "employment",
          limit: 200,
        },
      },
    });
    if (error) throwProblem(error);
    const row = data.data.find((row) => row.id === employment.relationship_id);
    if (!row)
      throwProblem({ detail: t("contact.rail.employmentVersionUnresolved") });
    return row;
  };
  const original = {
    ...employment,
    id: employment.relationship_id,
    started_at: employment.started_at?.slice(0, 10),
    ended_at: employment.ended_at?.slice(0, 10),
  };
  // Older snapshots may lack a version. Compare visible fields before using
  // their fresh version; never pin an unseen change as the user's baseline.
  if (original.version === undefined) {
    const fresh = await read();
    for (const key of [
      "role",
      "started_at",
      "ended_at",
      "employment_status",
    ] as const) {
      if (!sameEditValue(original[key], fresh[key]))
        throwProblem({ code: "version_skew" });
    }
    original.version = fresh.version;
  }
  await saveIndependentEdit({
    opened: { id: original.id, original },
    patch: body,
    groups: [
      ["started_at", "started_precision", "clear_started_at"],
      [
        "ended_at",
        "ended_precision",
        "clear_ended_at",
        "employment_status",
        "is_current_primary",
      ],
    ],
    read,
    write: async (patch, version) => {
      const { data, error } = await api.PATCH("/relationships/{id}", {
        params: { path: { id: original.id }, ...ifMatch(version) },
        body: patch,
      });
      if (error) throwProblem(error);
      return data;
    },
  });
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
  const [editing, setEditing] = useState<Employment | null>(null);
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
              canEdit={canEdit}
              readOnlyReason={readOnlyReason}
              actions={actions}
              onRemove={() => setRemoving(employment)}
              onEdit={() => setEditing(employment)}
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
        {editing && (
          <EmploymentEdit
            key={contact.id + editing.relationship_id}
            employment={editing}
            contactId={contact.id}
            onClose={() => setEditing(null)}
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
  onEdit,
  showCompany = true,
}: Readonly<{
  employment: Employment;
  showCompany?: boolean;
  canEdit: boolean;
  readOnlyReason: string | undefined;
  actions: EmploymentActions;
  onRemove: () => void;
  onEdit: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const detail = employmentDetail(employment, t, locale, zone);
  const ending =
    actions.end.isPending &&
    actions.end.variables?.relationship_id === employment.relationship_id;
  // A settled mutation is no longer pending. Match errors by relationship
  // so only the failed row keeps its message after the mutation settles.
  const endFailed =
    actions.end.isError &&
    actions.end.variables?.relationship_id === employment.relationship_id;
  return (
    <div className="pe-employment">
      <span className="pe-employment-body">
        <span className="pe-employment-company">
          {showCompany &&
            (employment.company_name ? (
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
            ))}
          {stillHeld(employment) && (
            <span className="pe-rail-value-good">{t("rel.current")}</span>
          )}
          {!stillHeld(employment) && (
            <span className="t-caption">
              {t(
                employment.employment_status === "unknown"
                  ? "employment.status.unknown"
                  : "employment.status.former",
              )}
            </span>
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
            <Button small onClick={onEdit}>
              {t("employment.edit")}
            </Button>
            {stillHeld(employment) && (
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
  zone: string,
): string {
  // Career dates keep the year and the precision actually recorded.
  const start = employment.started_at
    ? employment.started_precision === "month"
      ? employment.started_at.slice(0, 7)
      : formatDateAbbrev(employment.started_at.slice(0, 10), locale, zone)
    : undefined;
  const end = employment.ended_at
    ? employment.ended_precision === "month"
      ? employment.ended_at.slice(0, 7)
      : formatDateAbbrev(employment.ended_at.slice(0, 10), locale, zone)
    : undefined;
  if (start && end) {
    return `${start} – ${end}`;
  }
  if (end) {
    return t("rel.endedOn", { when: end });
  }
  if (start) {
    return `${start} – ${t(stillHeld(employment) ? "employment.status.current" : employment.employment_status === "unknown" ? "employment.status.unknown" : "employment.status.former")}`;
  }
  return "";
}
