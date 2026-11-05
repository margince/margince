import {
  type QueryClient,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Trash2 } from "lucide-react";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch, requireVersion } from "../../api/version";
import { type CreateField, CreateRecordModal } from "../../components/create";
import { EditAction } from "../../components/edit";
import { Badge, Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { IconAction } from "../../design-system/iconaction";
import { KeyCode } from "../../design-system/keyedname";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { useT } from "../../i18n";
import { useLegalReviewAvailable } from "../../lib/legalreviews.queries";
import { problemMessageOf, unwrap } from "../../lib/problem";
import {
  CRITERION_KIND_LABEL,
  CRITERION_KINDS,
  type CriterionKind,
  criterionKindOf,
} from "./criterionkinds";
import "./settings.pipelines.css";

type Criterion = components["schemas"]["StageExitCriterion"];

// The key an extractor cites. Spelled here as well as in the column's CHECK
// and the store's own guard, because a round trip to learn that a key may not
// start with a digit is a wait the reader need not pay. The server stays the
// authority: this only saves the trip.
const KEY_SHAPE = /^[a-z][a-z0-9_]{0,63}$/;

// The kinds that name something the BUYER did. A criterion of one of these is
// never settled by a message our own side wrote, which is the rule the callout
// states in prose — kept as data here so the two cannot drift.
const BUYER_KINDS: readonly CriterionKind[] = [
  "buyer_confirmed",
  "event_held",
  "document_signed",
  "terms_accepted",
];

/** A stage's exit criteria, inside the stage's row on the pipelines card. */
export function StageExitCriteria({
  stageId,
  stageName,
  canEdit,
}: Readonly<{
  stageId: string;
  stageName: string;
  canEdit: boolean;
}>) {
  const t = useT();
  const query = useQuery({
    queryKey: criteriaKey(stageId),
    queryFn: async () => {
      const data = unwrap(
        await api.GET("/stages/{id}/exit-criteria", {
          params: { path: { id: stageId }, query: {} },
        }),
      );
      return data.data;
    },
  });

  // Sorted here as well as in the query, exactly as PipelineRow sorts its
  // stages: position is what the reader is promised, and a list that depended
  // on the server's order would reorder itself the day a caching layer or a
  // second reader hands them back differently.
  const criteria = [...(query.data ?? [])].sort(
    (a, b) => a.position - b.position,
  );
  const settled = !query.isPending && !query.isError;
  return (
    <div className="stage-criteria">
      {criteria.some((c) => BUYER_KINDS.includes(c.kind)) && (
        <Callout
          tone="info"
          kind="standing"
          title={t("stage.criteria.buyerCalloutTitle")}
        >
          {t("stage.criteria.buyerCallout")}
        </Callout>
      )}
      {query.isError && (
        <Callout
          tone="danger"
          kind="outcome"
          title={t("stage.criteria.unreadableTitle")}
        >
          {/* The server's own account of the failure, the way every sibling
              read on this card reports one — a fixed sentence here would throw
              away the one detail that says whether reloading can help. */}
          {/* ds:ignore the cause inside a Callout's body; the Callout is the notice */}
          <p>{problemMessageOf(query.error, t)}</p>
          <p>{t("stage.criteria.unreadable")}</p>
        </Callout>
      )}
      {criteria.length > 0 && (
        <SettingList>
          {criteria.map((criterion) => (
            <CriterionRow
              key={criterion.id}
              criterion={criterion}
              stageId={stageId}
              canEdit={canEdit}
            />
          ))}
        </SettingList>
      )}
      {/* "Nothing yet" is a claim about the stage, so it is only honest once
          the read has actually answered. A failed read says so above instead:
          a screen that reported an empty configuration after a 500 would have
          an admin adding criteria that are already there. */}
      {(canEdit || (settled && criteria.length === 0)) && (
        <div className="stage-criteria-foot">
          {settled && criteria.length === 0 && (
            <p>{t("stage.criteria.none")}</p>
          )}
          {canEdit && (
            <CriterionCreate stageId={stageId} stageName={stageName} />
          )}
        </div>
      )}
    </div>
  );
}

function CriterionRow({
  criterion,
  stageId,
  canEdit,
}: Readonly<{
  criterion: Criterion;
  stageId: string;
  canEdit: boolean;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  return (
    <SettingRow
      label={criterion.label}
      description={t(CRITERION_KIND_LABEL[criterion.kind])}
      value={
        <Badge tone={criterion.required ? "warning" : undefined}>
          {criterion.required
            ? t("stage.criteria.required")
            : t("stage.criteria.optional")}
        </Badge>
      }
      control={
        canEdit && (
          <span className="cell-actions">
            <EditAction<Criterion>
              label={t("stage.criteria.editOf", { name: criterion.label })}
              savedMessage={(saved) =>
                t("record.saveDone", { name: saved.label })
              }
              invalidate="pipelines"
              recordKey="stage"
              record={{
                id: criterion.id,
                version: criterion.version,
                label: criterion.label,
                kind: criterion.kind,
                required: String(criterion.required),
                hint: criterion.hint ?? "",
              }}
              fields={criterionEditFields(t, criterion)}
              update={async (values, _rows, opened) => {
                const saved = unwrap(
                  await api.PATCH("/stages/{id}/exit-criteria/{criterion_id}", {
                    params: {
                      path: { id: stageId, criterion_id: criterion.id },
                      // Pin the version the form opened on. A refetch may carry
                      // another editor's save, which the live version would overwrite.
                      ...ifMatch(requireVersion(opened?.version)),
                    },
                    body: criterionEditBody(values),
                  }),
                );
                // The edit form refreshes the pipelines; this list is read apart.
                queryClient.invalidateQueries({
                  queryKey: criteriaKey(stageId),
                });
                return saved;
              }}
            />
            <CriterionRemove criterion={criterion} stageId={stageId} />
          </span>
        )
      }
    />
  );
}

function CriterionCreate({
  stageId,
  stageName,
}: Readonly<{ stageId: string; stageName: string }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const stageNameId = useId();
  return (
    <>
      <span id={stageNameId} className="sr-only">
        {stageName}
      </span>
      <Button
        data-testid={`new-criterion-${stageId}`}
        aria-describedby={stageNameId}
        onClick={() => setOpen(true)}
      >
        {t("stage.criteria.add")}
      </Button>
      <CriterionCreateDialog
        stageId={stageId}
        open={open}
        onClose={() => setOpen(false)}
      />
    </>
  );
}

/** The new-criterion form on its own, for a verb that lives outside the editor. */
export function CriterionCreateDialog({
  stageId,
  open,
  onClose,
}: Readonly<{ stageId: string; open: boolean; onClose: () => void }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const legalReview = useLegalReviewAvailable();
  const mutation = useMutation({
    mutationFn: async (
      draft: Readonly<{ stageId: string; values: Record<string, string> }>,
    ) => {
      const { values } = draft;
      return unwrap(
        await api.POST("/stages/{id}/exit-criteria", {
          params: { path: { id: draft.stageId } },
          body: {
            key: values.key ?? "",
            label: values.label ?? "",
            kind: criterionKindOf(values.kind),
            required: values.required !== "false",
            ...(values.hint ? { hint: values.hint } : {}),
          },
        }),
      );
    },
    onSuccess: (_created, draft) => {
      onClose();
      refreshCriteria(queryClient, draft.stageId);
    },
  });
  return (
    <CreateRecordModal
      open={open}
      onClose={() => {
        mutation.reset();
        onClose();
      }}
      title={t("stage.criteria.add")}
      fields={criterionCreateFields(t, legalReview === true)}
      pending={mutation.isPending}
      error={mutation.isError ? problemMessageOf(mutation.error, t) : null}
      onSubmit={(values) => mutation.mutate({ stageId, values })}
    />
  );
}

// Archiving, not deleting — evidence recorded against the criterion stays
// readable, which is what the body says rather than promising a removal the
// server does not perform.
function CriterionRemove({
  criterion,
  stageId,
}: Readonly<{ criterion: Criterion; stageId: string }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: async (target: Criterion) => {
      unwrap(
        await api.DELETE("/stages/{id}/exit-criteria/{criterion_id}", {
          params: {
            path: { id: stageId, criterion_id: target.id },
            ...ifMatch(requireVersion(target.version)),
          },
        }),
      );
    },
    onSuccess: () => {
      setOpen(false);
      refreshCriteria(queryClient, stageId);
    },
  });
  return (
    <>
      <IconAction
        label={t("stage.criteria.removeOf", { name: criterion.label })}
        icon={<Trash2 aria-hidden />}
        opensDialog
        onClick={() => setOpen(true)}
      />
      <ConfirmModal
        open={open}
        onClose={() => setOpen(false)}
        title={t("stage.criteria.removeTitle")}
        confirmLabel={t("stage.criteria.remove")}
        confirmVariant="danger"
        onConfirm={() => mutation.mutate(criterion)}
        pending={mutation.isPending}
        error={mutation.isError ? problemMessageOf(mutation.error, t) : null}
      >
        <p>{t("stage.criteria.removeBody", { name: criterion.label })}</p>
      </ConfirmModal>
    </>
  );
}

// Not under ["pipelines"]: every stage reorder would refetch each open editor.
function criteriaKey(stageId: string) {
  return ["stage-exit-criteria", stageId] as const;
}

// A criterion write also moves the counts the stage table reads.
function refreshCriteria(queryClient: QueryClient, stageId: string) {
  queryClient.invalidateQueries({ queryKey: ["pipelines"] });
  queryClient.invalidateQueries({ queryKey: criteriaKey(stageId) });
}

// The server keeps a criterion from turning into, or out of, legal_reviewed,
// and offers the kind only while legal review is switched on.
function kindOptions(
  t: ReturnType<typeof useT>,
  admits: (kind: CriterionKind) => boolean,
) {
  return CRITERION_KINDS.filter(admits).map((kind) => ({
    value: kind,
    label: t(CRITERION_KIND_LABEL[kind]),
  }));
}

function requiredOptions(t: ReturnType<typeof useT>) {
  return [
    { value: "true", label: t("stage.criteria.required") },
    { value: "false", label: t("stage.criteria.optional") },
  ];
}

function criterionCreateFields(
  t: ReturnType<typeof useT>,
  legalReview: boolean,
): CreateField[] {
  return [
    {
      key: "key",
      label: "stage.criteria.key",
      required: true,
      hint: t("stage.criteria.keyHint"),
      validate: (value) =>
        KEY_SHAPE.test(value) ? undefined : t("stage.criteria.keyHint"),
    },
    { key: "label", label: "stage.criteria.label", required: true },
    ...criterionShapeFields(
      t,
      (kind) => legalReview || kind !== "legal_reviewed",
    ),
  ];
}

// The edit form shows `key` and never edits it: evidence matches a criterion by
// key, so renaming one would orphan every claim already recorded under it.
function criterionEditFields(
  t: ReturnType<typeof useT>,
  current: Pick<Criterion, "key" | "kind">,
): CreateField[] {
  const legal = current.kind === "legal_reviewed";
  return [
    {
      key: "label",
      label: "stage.criteria.label",
      required: true,
      hint: <KeyCode code={current.key} />,
    },
    ...criterionShapeFields(t, (kind) => (kind === "legal_reviewed") === legal),
  ];
}

// The three fields both forms share, spelled once.
function criterionShapeFields(
  t: ReturnType<typeof useT>,
  admits: (kind: CriterionKind) => boolean,
): CreateField[] {
  return [
    {
      key: "kind",
      label: "stage.criteria.kind",
      type: "select",
      required: true,
      options: kindOptions(t, admits),
    },
    {
      key: "required",
      label: "stage.criteria.required",
      type: "select",
      required: true,
      options: requiredOptions(t),
    },
    { key: "hint", label: "stage.criteria.hint", type: "textarea" },
  ];
}

// The form hands its values back as unknowns, so each one is narrowed here
// rather than asserted: a value that is not a string is an empty one, which is
// what the required-field guard already refused.
function text(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function criterionEditBody(values: Record<string, unknown>) {
  return {
    label: text(values.label),
    kind: criterionKindOf(text(values.kind)),
    required: text(values.required) !== "false",
    // An emptied hint is a CLEAR, which the contract spells as an explicit
    // null. Sending "" instead would store an empty string the reader cannot
    // tell from a hint nobody wrote.
    hint: text(values.hint) ? text(values.hint) : null,
  };
}
