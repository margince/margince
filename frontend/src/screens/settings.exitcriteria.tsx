import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { Badge, Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, throwProblem } from "./common";
import { type CreateField, CreateRecordModal } from "./create";
import { EditAction } from "./edit";

type Criterion = components["schemas"]["StageExitCriterion"];
type CriterionKind = components["schemas"]["StageCriterionKind"];

// The key an extractor cites. Spelled here as well as in the column's CHECK
// and the store's own guard, because a round trip to learn that a key may not
// start with a digit is a wait the reader need not pay. The server stays the
// authority: this only saves the trip.
const KEY_SHAPE = /^[a-z][a-z0-9_]{0,63}$/;

// Every kind the contract admits, with the label a reader sees. Derived from
// the generated union rather than retyped, so a kind added to crm.yaml and
// missing here fails the typecheck instead of rendering as a raw enum value.
const KIND_LABEL: Record<CriterionKind, MessageKey> = {
  buyer_confirmed: "stage.criteria.kindBuyerConfirmed",
  event_held: "stage.criteria.kindEventHeld",
  document_signed: "stage.criteria.kindDocumentSigned",
  role_identified: "stage.criteria.kindRoleIdentified",
  terms_accepted: "stage.criteria.kindTermsAccepted",
  custom: "stage.criteria.kindCustom",
};

// The kinds that name something the BUYER did. A criterion of one of these is
// never settled by a message our own side wrote, which is the rule the callout
// states in prose — kept as data here so the two cannot drift.
const BUYER_KINDS: readonly CriterionKind[] = [
  "buyer_confirmed",
  "event_held",
  "document_signed",
  "terms_accepted",
];

/**
 * A stage's exit criteria, inside the stage's row on the pipelines card.
 *
 * Terminal stages are the interesting case: won and lost are where a deal
 * stops, so the section says so and offers no Add. That is the server's rule
 * too, and asking it first would still be right — but a control that only
 * exists to be refused is worse than one that is never drawn.
 */
export function StageExitCriteria({
  stageId,
  semantic,
  canEdit,
}: Readonly<{
  stageId: string;
  semantic: components["schemas"]["Stage"]["semantic"];
  canEdit: boolean;
}>) {
  const t = useT();
  const terminal = semantic === "won" || semantic === "lost";
  const query = useQuery({
    queryKey: ["stage-exit-criteria", stageId],
    queryFn: async () => {
      const { data, error } = await api.GET("/stages/{id}/exit-criteria", {
        params: { path: { id: stageId }, query: {} },
      });
      if (error) {
        throwProblem(error);
      }
      return data.data;
    },
    // A terminal stage can hold none, so the read is not worth a round trip.
    enabled: !terminal,
  });

  if (terminal) {
    return <p className="settings-panel-sub">{t("stage.criteria.terminal")}</p>;
  }
  // Sorted here as well as in the query, exactly as PipelineRow sorts its
  // stages: position is what the reader is promised, and a list that depended
  // on the server's order would reorder itself the day a caching layer or a
  // second reader hands them back differently.
  const criteria = [...(query.data ?? [])].sort(
    (a, b) => a.position - b.position,
  );
  return (
    <div className="form-stack">
      <p className="settings-panel-sub">{t("stage.criteria.sub")}</p>
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
          <p>{problemMessageOf(query.error, t)}</p>
          <p>{t("stage.criteria.unreadable")}</p>
        </Callout>
      )}
      {/* "Nothing yet" is a claim about the stage, so it is only honest once
          the read has actually answered. A failed read says so above instead:
          a screen that reported an empty configuration after a 500 would have
          an admin adding criteria that are already there. */}
      {criteria.length === 0 && !query.isPending && !query.isError && (
        <p className="settings-panel-sub">{t("stage.criteria.none")}</p>
      )}
      <ul className="criterion-rows">
        {criteria.map((criterion) => (
          <CriterionRow
            key={criterion.id}
            criterion={criterion}
            stageId={stageId}
            canEdit={canEdit}
          />
        ))}
      </ul>
      {canEdit && <CriterionCreate stageId={stageId} />}
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
  return (
    <li className="criterion-row">
      <span className="criterion-label">{criterion.label}</span>
      <span className="t-mono t-caption">{criterion.key}</span>
      <Badge>{t(KIND_LABEL[criterion.kind])}</Badge>
      <Badge tone={criterion.required ? "warn" : undefined}>
        {criterion.required
          ? t("stage.criteria.required")
          : t("stage.criteria.optional")}
      </Badge>
      {canEdit && (
        <span className="criterion-verbs">
          <EditAction<Criterion>
            label={t("stage.criteria.edit")}
            savedMessage={(saved) =>
              t("record.saveDone", { name: saved.label })
            }
            invalidate="stage-exit-criteria"
            recordKey="stage"
            record={{
              id: criterion.id,
              version: criterion.version,
              label: criterion.label,
              kind: criterion.kind,
              required: String(criterion.required),
              hint: criterion.hint ?? "",
            }}
            fields={criterionEditFields(t)}
            update={async (values, _rows, opened) => {
              const { data, error } = await api.PATCH(
                "/stages/{id}/exit-criteria/{criterion_id}",
                {
                  params: {
                    path: { id: stageId, criterion_id: criterion.id },
                    // The version the form OPENED on, not the one the row
                    // carries now. A background refetch can pull in another
                    // editor's save while this modal stays open; pinning the
                    // live version would then submit these stale values
                    // against their version and overwrite them.
                    ...ifMatch(requireVersion(opened?.version)),
                  },
                  body: criterionEditBody(values),
                },
              );
              if (error) {
                throwProblem(error);
              }
              return data;
            }}
          />
          <CriterionRemove criterion={criterion} stageId={stageId} />
        </span>
      )}
    </li>
  );
}

function CriterionCreate({ stageId }: Readonly<{ stageId: string }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: async (values: Record<string, string>) => {
      const { data, error } = await api.POST("/stages/{id}/exit-criteria", {
        params: { path: { id: stageId } },
        body: {
          key: values.key,
          label: values.label,
          kind: values.kind as CriterionKind,
          required: values.required !== "false",
          ...(values.hint ? { hint: values.hint } : {}),
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => {
      setOpen(false);
      queryClient.invalidateQueries({ queryKey: ["stage-exit-criteria"] });
    },
  });
  return (
    <>
      <Button
        small
        data-testid={`new-criterion-${stageId}`}
        onClick={() => setOpen(true)}
      >
        {t("stage.criteria.add")}
      </Button>
      <CreateRecordModal
        open={open}
        onClose={() => setOpen(false)}
        title={t("stage.criteria.add")}
        fields={criterionCreateFields(t)}
        pending={mutation.isPending}
        error={mutation.isError ? problemMessageOf(mutation.error, t) : null}
        onSubmit={(values) => mutation.mutate(values)}
      />
    </>
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
    mutationFn: async () => {
      const { error } = await api.DELETE(
        "/stages/{id}/exit-criteria/{criterion_id}",
        {
          params: {
            path: { id: stageId, criterion_id: criterion.id },
            ...ifMatch(requireVersion(criterion.version)),
          },
        },
      );
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      setOpen(false);
      queryClient.invalidateQueries({ queryKey: ["stage-exit-criteria"] });
    },
  });
  return (
    <>
      <Button small variant="ghost" onClick={() => setOpen(true)}>
        {t("stage.criteria.remove")}
      </Button>
      <ConfirmModal
        open={open}
        onClose={() => setOpen(false)}
        title={t("stage.criteria.removeTitle")}
        confirmLabel={t("stage.criteria.remove")}
        confirmVariant="danger"
        onConfirm={() => mutation.mutate()}
        pending={mutation.isPending}
        error={mutation.isError ? problemMessageOf(mutation.error, t) : null}
      >
        <p>{t("stage.criteria.removeBody", { name: criterion.label })}</p>
      </ConfirmModal>
    </>
  );
}

function kindOptions(t: ReturnType<typeof useT>) {
  return (Object.keys(KIND_LABEL) as CriterionKind[]).map((kind) => ({
    value: kind,
    label: t(KIND_LABEL[kind]),
  }));
}

function requiredOptions(t: ReturnType<typeof useT>) {
  return [
    { value: "true", label: t("stage.criteria.required") },
    { value: "false", label: t("stage.criteria.optional") },
  ];
}

function criterionCreateFields(t: ReturnType<typeof useT>): CreateField[] {
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
    ...criterionShapeFields(t),
  ];
}

// The edit form omits `key`: evidence matches a criterion by key across an
// edit, so renaming one would orphan every claim already recorded under it.
// The contract has no key field on the PATCH body either.
function criterionEditFields(t: ReturnType<typeof useT>): CreateField[] {
  return [
    { key: "label", label: "stage.criteria.label", required: true },
    ...criterionShapeFields(t),
  ];
}

// The three fields both forms share, spelled once.
function criterionShapeFields(t: ReturnType<typeof useT>): CreateField[] {
  return [
    {
      key: "kind",
      label: "stage.criteria.kind",
      type: "select",
      required: true,
      options: kindOptions(t),
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
    kind: text(values.kind) as CriterionKind,
    required: text(values.required) !== "false",
    // An emptied hint is a CLEAR, which the contract spells as an explicit
    // null. Sending "" instead would store an empty string the reader cannot
    // tell from a hint nobody wrote.
    hint: text(values.hint) ? text(values.hint) : null,
  };
}
