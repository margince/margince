import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  Button,
  Disclosure,
  EmptyState,
  Modal,
} from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { KeyCode } from "../../design-system/keyedname";
import { NameDialog } from "../../design-system/namedialog";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryStates } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { ANALYTICS_SCHEMA_KEY } from "../../lib/analyticskeys";
import { useCan, useCanWrite } from "../../lib/capability";
import {
  CF_OBJECTS,
  type CfObject,
  type CfType,
  columnName,
  slug,
} from "../../lib/customfields.logic";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";
import { FieldBuilder, type NewFieldDraft } from "./customfields.builder";
import { objectLabels } from "./customfields.labels";
import { RetireFieldConfirm } from "./customfields.retire";
import {
  AuditRail,
  auditState,
  type CustomField,
  type FieldSetting,
  FieldTable,
  SETTING_TOAST,
  STAGED_ID,
} from "./customfields.table";
import "./customfields.css";

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
    show_in_create: draft.showInCreate,
    ...(draft.type === "currency" ? { currency: draft.currency } : {}),
    ...(hasOptions(draft.type) ? { options: cleanOptions(draft.options) } : {}),
  };
}

// A record field keeps the kind it points at in its single option.
function hasOptions(type: CfType): boolean {
  return type === "picklist" || type === "multiselect" || type === "record";
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
    options: hasOptions(draft.type) ? cleanOptions(draft.options) : null,
    show_in_create: draft.showInCreate,
    reportable: false,
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
    queryClient.invalidateQueries({ queryKey: ANALYTICS_SCHEMA_KEY });
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

  // The variable carries the field, so a flip that lands before React re-arms
  // the mutation still writes the row the reader pressed. The saved row goes
  // straight into the list of the field's own object. A refetch keyed by the
  // object picked now could miss it, and the next flip would send a stale
  // version.
  const setting = useMutation({
    mutationFn: async (input: {
      field: CustomField;
      setting: FieldSetting;
      on: boolean;
    }) => {
      return unwrap(
        await api.PATCH("/custom-fields/{id}", {
          params: {
            path: { id: input.field.id },
            header: input.field.version
              ? { "If-Match": String(input.field.version) }
              : undefined,
          },
          body: { [input.setting]: input.on },
        }),
      );
    },
    onSuccess: (saved, input) => {
      queryClient.setQueryData<CustomFieldList>(
        ["custom-fields", input.field.object],
        (old) =>
          old && {
            ...old,
            data: old.data.map((one) => (one.id === saved.id ? saved : one)),
          },
      );
      queryClient.invalidateQueries({ queryKey: ["cf-audit"] });
      queryClient.invalidateQueries({ queryKey: ANALYTICS_SCHEMA_KEY });
      toast.show(
        t(SETTING_TOAST[input.setting][input.on ? "on" : "off"], {
          label: input.field.label,
        }),
      );
    },
    onError: (error) => {
      toast.show(problemMessageOf(error, t), { tone: "danger" });
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
          onSetting={(field, name, on) =>
            setting.mutate({ field, setting: name, on })
          }
          savingField={
            setting.isPending ? setting.variables?.field.id : undefined
          }
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
