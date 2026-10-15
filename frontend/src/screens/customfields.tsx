import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { X } from "lucide-react";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import {
  Badge,
  Button,
  Disclosure,
  EmptyState,
  Field,
  Modal,
  SegmentedControl,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Heading } from "../design-system/heading";
import { KeyCode } from "../design-system/keyedname";
import { NameDialog } from "../design-system/namedialog";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { QueryStates } from "../design-system/querystates";
import { Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { useToast } from "../design-system/toast";
import { AutonomyDot } from "../design-system/trust";
import { useT } from "../i18n";
import { useCan, useCanWrite } from "../lib/capability";
import {
  apiKey,
  CF_OBJECTS,
  CF_TYPES,
  type CfObject,
  type CfType,
  columnName,
  ddlPreview,
  looksStructural,
  slug,
} from "../lib/customfields.logic";
import { problemMessageOf, unwrap } from "../lib/problem";
import { useMe } from "../lib/session";
import { objectLabels, typeLabels } from "./customfields.labels";
import { RetireFieldConfirm } from "./customfields.retire";
import {
  AuditRail,
  auditState,
  type CustomField,
  FieldTable,
  STAGED_ID,
} from "./customfields.table";
import "./customfields.css";

// The add-field builder (AC-custom-fields-3..5/8): a governed form that turns a
// human's plain label into one typed scalar column on an existing object. The
// immutable cf_-prefixed API key and the pending DDL are shown before Confirm so
// the schema change is legible, a structural-sounding label is refused up front,
// and the 🟡 gate states that Confirm writes a live column + an audit row. This
// is NOT the Accept/Edit/Dismiss triad — it is a `warning` Callout,
// which is what the surface saying something about itself already looks like
// everywhere else.

export type NewFieldDraft = {
  object: CfObject;
  label: string;
  type: CfType;
  currency: string;
  options: string[];
};

export function FieldBuilder({
  object,
  pending,
  onSubmit,
  onCancel,
}: Readonly<{
  object: CfObject;
  pending: boolean;
  onSubmit: (draft: NewFieldDraft) => void;
  // Leaving the form without adding a field. The builder lives in a dialog, so
  // its secondary verb is what closes that dialog rather than what empties the
  // inputs: a form that is discarded on close has nothing to reset to.
  onCancel: () => void;
}>) {
  const toast = useToast();
  const t = useT();
  const [label, setLabel] = useState("");
  const [type, setType] = useState<CfType>("text");
  const [currency, setCurrency] = useState("EUR");
  const [options, setOptions] = useState<string[]>([""]);
  const structural = looksStructural(label);
  // A picklist with no non-blank option is not a picklist, and a currency field
  // needs a well-formed 3-letter ISO-4217 code — Confirm stays disabled until
  // the type-specific shape is valid, not just the label.
  const typeShapeValid =
    ((type !== "picklist" && type !== "multiselect") ||
      options.some((opt) => opt.trim().length > 0)) &&
    (type !== "currency" || /^[A-Za-z]{3}$/.test(currency.trim()));
  const canConfirm =
    !pending && label.trim().length > 0 && !structural && typeShapeValid;

  const setOptionAt = (idx: number, value: string) => {
    setOptions((current) => current.map((opt, i) => (i === idx ? value : opt)));
  };

  const removeOption = (idx: number) => {
    // A picklist without an option is not a picklist — the last row is a floor,
    // not a delete target, so the intent is surfaced as a toast, not swallowed.
    if (options.length <= 1) {
      // `danger`: this is a refusal, and the completion dot the default tone
      // draws said the opposite of what the sentence says.
      toast.show(t("cf.lastOptionBlocked"), { tone: "danger" });
      return;
    }
    setOptions((current) => current.filter((_, i) => i !== idx));
  };

  const confirm = () => {
    if (!canConfirm) {
      return;
    }
    onSubmit({ object, label: label.trim(), type, currency, options });
  };

  return (
    <>
      <div className="form-stack">
        <div className="cf-builder-head">
          <p className="cf-hint">{t("cf.builder.intro")}</p>
          <Badge>{t("cf.builder.noCode")}</Badge>
        </div>

        <div className="cf-grid">
          <Field label={t("cf.label")}>
            {(control) => (
              <TextInput
                {...control}
                value={label}
                onChange={(event) => setLabel(event.target.value)}
              />
            )}
          </Field>
          <Field label={t("cf.apiKey")} hint={t("cf.apiKeyHint")}>
            {(control) => (
              <TextInput
                {...control}
                value={apiKey(object, label)}
                disabled
                readOnly
              />
            )}
          </Field>
        </div>

        <div className="field">
          <span className="t-label">{t("cf.typeLabel")}</span>
          <SegmentedControl
            label={t("cf.typeLabel")}
            options={CF_TYPES}
            value={type}
            onChange={setType}
            labels={typeLabels(t)}
          />
        </div>

        {type === "currency" && (
          <Field label={t("cf.currencyCode")} hint={t("cf.currencyHint")}>
            {(control) => (
              <TextInput
                {...control}
                value={currency}
                maxLength={3}
                onChange={(event) =>
                  setCurrency(event.target.value.toUpperCase())
                }
              />
            )}
          </Field>
        )}

        {(type === "picklist" || type === "multiselect") && (
          <div className="field">
            <span className="t-label">{t("cf.options")}</span>
            <div className="cf-options">
              {options.map((option, idx) => (
                // biome-ignore lint/suspicious/noArrayIndexKey: option rows are positional, not identity-keyed
                <div className="cf-option-row" key={idx}>
                  <TextInput
                    aria-label={t("cf.optionPlaceholder")}
                    placeholder={t("cf.optionPlaceholder")}
                    value={option}
                    onChange={(event) => setOptionAt(idx, event.target.value)}
                  />
                  <Button
                    iconOnly
                    aria-label={t("cf.removeOption")}
                    onClick={() => removeOption(idx)}
                  >
                    <X aria-hidden="true" />
                  </Button>
                </div>
              ))}
            </div>
            <Button onClick={() => setOptions((current) => [...current, ""])}>
              {t("cf.addOption")}
            </Button>
          </div>
        )}

        {structural && (
          <Callout tone="danger" kind="standing" title={t("cf.refuse.title")}>
            <p>{t("cf.refuse.body")}</p>
            <p>{t("cf.refuse.route")}</p>
          </Callout>
        )}

        {/* `warning`: confirmed unread, the column lands on every record. */}
        <Callout
          tone="warning"
          kind="standing"
          title={
            <>
              <AutonomyDot tier="confirm" /> {t("cf.gate.title")}
            </>
          }
        >
          <p>{t("cf.gate.body", { object: t(`cf.obj.${object}`) })}</p>
          <code className="cf-ddl t-caption">
            {ddlPreview(object, label, type, currency)}
          </code>
        </Callout>
      </div>

      {/* Cancel first, then the verb that writes — the order every dialog in
          this tree uses, so the destructive-looking half is never where the
          reader's hand expects the safe one. Reset went with the disclosure
          this form used to live in: closing the dialog discards the draft, so
          a control that empties the inputs in place has nothing left to do. */}
      <div className="actions">
        <Button variant="ghost" onClick={onCancel}>
          {t("deals.cancel")}
        </Button>
        <Button variant="primary" disabled={!canConfirm} onClick={confirm}>
          {t("cf.confirm")}
        </Button>
      </div>
    </>
  );
}

type CustomFieldList = components["schemas"]["CustomFieldListResponse"];

// The add-field create body (CUSTOM-FIELDS-WIRE-2): a plain manual field carries
// `source:"manual"` (the FE convention across deals/leads/companies), and the
// two conditional shapes ride only on their own type — currency on a currency
// field, options on a picklist — never on the others.
function createBody(
  draft: NewFieldDraft,
): components["schemas"]["CreateCustomFieldRequest"] {
  return {
    object: draft.object,
    label: draft.label,
    type: draft.type,
    source: "manual",
    ...(draft.type === "currency" ? { currency: draft.currency } : {}),
    ...(draft.type === "picklist" || draft.type === "multiselect"
      ? { options: cleanOptions(draft.options) }
      : {}),
  };
}

// A picklist ships only its real choices: blank / whitespace-only rows (the
// editor's floor row and any half-typed entries) are dropped and exact
// duplicates collapsed, so the stored enum matches what the admin sees.
function cleanOptions(options: string[]): string[] {
  const seen = new Set<string>();
  const cleaned: string[] = [];
  for (const raw of options) {
    const trimmed = raw.trim();
    if (trimmed.length === 0 || seen.has(trimmed)) {
      continue;
    }
    seen.add(trimmed);
    cleaned.push(trimmed);
  }
  return cleaned;
}

// The optimistic row shown while the create is in flight — a full CustomField so
// the table renders it, tagged with STAGED_ID so it gets the pending treatment
// (no rename/archive affordance) and is rolled back on error.
function stagedField(draft: NewFieldDraft, createdBy: string): CustomField {
  const now = new Date().toISOString();
  return {
    id: STAGED_ID,
    object: draft.object,
    label: draft.label,
    slug: slug(draft.label),
    type: draft.type,
    status: "active",
    column_name: columnName(draft.label),
    currency: draft.type === "currency" ? draft.currency : null,
    options:
      draft.type === "picklist" || draft.type === "multiselect"
        ? cleanOptions(draft.options)
        : null,
    created_by: createdBy,
    created_at: now,
    updated_at: now,
  };
}

// Content inside a settings page, which owns the reading column and the h1. One
// Panel whose h2 sits under that h1 keeps the page to one title.
export function CustomFieldsAdmin() {
  const t = useT();
  const queryClient = useQueryClient();
  const me = useMe();
  // Two grants, two affordances: the builder adds a column to a live table,
  // while rename and retire change one that already exists.
  const canCreate = useCanWrite("custom_field", "create");
  const canEdit = useCanWrite("custom_field", "update");
  // The trail is not this screen's: /audit-log asks for `audit_log:read`
  // (privacy.ListAuditLog), while this page opens for anyone holding
  // custom_field:read. So the rail below asks for the trail's own grant.
  const readsAuditTrail = useCan("audit_log", "read");
  const meUserId = me.data?.user?.id;

  const [object, setObject] = useState<CfObject>("deal");
  const toast = useToast();
  const [renaming, setRenaming] = useState<CustomField | null>(null);
  const [retiring, setRetiring] = useState<CustomField | null>(null);
  // The dialog stays MOUNTED so it can animate out, so `addSeq` is what gives
  // each open a builder of its own: it re-keys the form, which discards a
  // half-typed label rather than leaving it waiting under an object nobody
  // re-chose, and stops a second Confirm resubmitting a draft already created.
  const [adding, setAdding] = useState(false);
  const [addSeq, setAddSeq] = useState(0);
  const addId = useId();

  const list = useQuery({
    queryKey: ["custom-fields", object],
    queryFn: async () => {
      return unwrap(
        await api.GET("/custom-fields", {
          params: { query: { object } },
        }),
      );
    },
  });

  const audit = useQuery({
    queryKey: ["cf-audit"],
    // A denial that is already known is not worth a request. Without this a
    // reader without the grant fired a call that could only 403 and got the
    // rail's red role="alert" back — a failure with a retry that can never
    // succeed, over a refusal they cannot act on.
    enabled: readsAuditTrail,
    queryFn: async () => {
      return unwrap(
        await api.GET("/audit-log", {
          params: { query: { entity_type: "custom_field" } },
        }),
      );
    },
  });

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["custom-fields", object] });
    queryClient.invalidateQueries({ queryKey: ["cf-audit"] });
  };

  const create = useMutation({
    mutationFn: async (draft: NewFieldDraft) => {
      return unwrap(
        await api.POST("/custom-fields", {
          body: createBody(draft),
        }),
      );
    },
    onMutate: async (draft: NewFieldDraft) => {
      // Key the optimistic write to the DRAFT's object, not the current-render
      // one, so switching objects mid-create still stages, rolls back, and
      // invalidates the right list (m2).
      const key = ["custom-fields", draft.object];
      await queryClient.cancelQueries({ queryKey: key });
      const previous = queryClient.getQueryData<CustomFieldList>(key);
      queryClient.setQueryData<CustomFieldList>(key, (old) =>
        old
          ? { ...old, data: [...old.data, stagedField(draft, meUserId ?? "")] }
          : old,
      );
      return { previous, key };
    },
    onError: (error, _draft, context) => {
      if (context) {
        queryClient.setQueryData(context.key, context.previous);
      }
      toast.show(problemMessageOf(error, t), { tone: "danger" });
    },
    onSuccess: (_data, draft) => {
      queryClient.invalidateQueries({
        queryKey: ["custom-fields", draft.object],
      });
      queryClient.invalidateQueries({ queryKey: ["cf-audit"] });
      toast.show(t("cf.added", { label: draft.label }));
      // The dialog goes, and the toast is what reports the outcome on the card
      // behind it — a reader who has just added a column has no second one to
      // type into, and unmounting the form is what clears the committed draft.
      setAdding(false);
    },
  });

  const rename = useMutation({
    mutationFn: async (input: { field: CustomField; label: string }) => {
      return unwrap(
        await api.PATCH("/custom-fields/{id}", {
          params: {
            path: { id: input.field.id },
            header: input.field.version
              ? { "If-Match": String(input.field.version) }
              : undefined,
          },
          body: { label: input.label },
        }),
      );
    },
    onSuccess: (_data, input) => {
      invalidate();
      toast.show(t("cf.renamed", { label: input.label }));
      setRenaming(null);
    },
  });

  const startRename = (field: CustomField) => {
    rename.reset();
    setRenaming(field);
  };

  const objectName = t(`cf.obj.${object}`);
  const listed = list.isSuccess && list.data.data.length > 0;

  return (
    <Panel
      title={t("cf.title")}
      // The create verb is the card's, so it stands in the header band. As a
      // trailing row its label ("Add a field to Deal") said the same thing as
      // the button beside it, which is one act named twice on one line; the
      // object it applies to is chosen in the row below and named again by the
      // dialog. Absent without the create grant, as the row was: a
      // surface that is only an action makes no claim about the data by not
      // being there.
      titleAction={
        canCreate && (
          <Button
            onClick={() => {
              setAddSeq((seq) => seq + 1);
              setAdding(true);
            }}
          >
            {t("cf.builder.open")}
          </Button>
        )
      }
    >
      {canCreate && (
        <PanelBody>
          <PanelIntro>{t("cf.subtitle")}</PanelIntro>
        </PanelBody>
      )}
      <SettingList bleed="settings">
        <SettingRow
          label={t("cf.object")}
          control={(control) => (
            <Select
              {...control}
              options={CF_OBJECTS.map((one) => ({
                value: one,
                label: objectLabels(t)[one],
              }))}
              value={object}
              onChange={(next) =>
                setObject(CF_OBJECTS.find((one) => one === next) ?? object)
              }
            />
          )}
        />
      </SettingList>
      {!listed && (
        <PanelBody>
          <QueryStates
            query={list}
            pendingLabel={t("cf.listLabel", { object: objectName })}
          >
            <EmptyState>
              {canCreate
                ? t(`cf.empty.${object}`)
                : t("cf.emptyReadOnly", { object: objectName })}
            </EmptyState>
          </QueryStates>
        </PanelBody>
      )}
      {listed && (
        <FieldTable
          object={object}
          fields={list.data.data}
          canEdit={canEdit}
          meUserId={meUserId}
          onRename={startRename}
          onArchive={setRetiring}
        />
      )}
      <PanelBody>
        {/* Withheld, not absent: the trail keeps its place for every reader,
            because a section that simply were not there would read as "nobody
            has changed a field" — a claim about the data in place of one about
            who may read it. Closed by default because it is a secondary read;
            the state inside it is settled before it is ever opened. */}
        <Disclosure summary={t("cf.audit.title")}>
          <AuditRail
            entries={audit.data?.data ?? []}
            state={auditState(
              readsAuditTrail,
              audit.isPending,
              audit.isError,
              audit.data?.data.length ?? 0,
            )}
            meUserId={meUserId}
            onRetry={() => void audit.refetch()}
          />
          {/* True for every reader: the recording happens whether or not this
              one may read it back. */}
          <p className="cf-hint t-caption">{t("cf.audit.footer")}</p>
        </Disclosure>
      </PanelBody>

      <Modal
        open={adding}
        intent="form"
        onClose={() => setAdding(false)}
        labelledBy={addId}
      >
        <Heading size="large" id={addId} className="t-h2 modal-title">
          {t("cf.builder.addTo", { object: objectName })}
        </Heading>
        <FieldBuilder
          key={addSeq}
          object={object}
          pending={create.isPending}
          onSubmit={(draft) => create.mutate(draft)}
          onCancel={() => setAdding(false)}
        />
      </Modal>

      <RetireFieldConfirm
        field={retiring}
        onClose={() => setRetiring(null)}
        onRetired={(field) => {
          invalidate();
          toast.show(t("cf.archived", { label: field.label }));
          setRetiring(null);
        }}
      />

      <NameDialog
        open={renaming !== null}
        onClose={() => setRenaming(null)}
        title={t("cf.edit")}
        label={t("cf.renamePrompt")}
        initial={renaming?.label ?? ""}
        hint={
          renaming && (
            <KeyCode code={`${renaming.object}.${renaming.column_name}`} />
          )
        }
        confirmLabel={t("trust.save")}
        pending={rename.isPending}
        problem={rename.isError ? problemMessageOf(rename.error, t) : null}
        onSave={(label) => {
          if (renaming) rename.mutate({ field: renaming, label });
        }}
      />
    </Panel>
  );
}
