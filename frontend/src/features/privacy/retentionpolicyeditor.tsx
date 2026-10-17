import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import { Button, Field, Modal, TextInput } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Select } from "../../design-system/select";
import { Switch } from "../../design-system/switch";
import { useT } from "../../i18n";
import { problemMessageOf, unwrap } from "../../lib/problem";
import {
  actionLabelKey,
  parseRetainDays,
  RETENTION_ACTIONS,
  RETENTION_POLICIES_KEY,
  type RetentionAction,
  type RetentionPolicy,
  scopeLabelKey,
} from "./retention.logic";

// A row's two writes are one PATCH on one policy, so they stay one mutation.
// `intent` says which of the two it is, because only a save closes the editor.
type PolicyWrite = Readonly<{
  intent: "save" | "switch";
  body: Readonly<{
    retain_days?: number;
    action?: RetentionAction;
    lawful_basis?: string | null;
    enabled?: boolean;
  }>;
}>;

// Mounted once per opening, keyed on the session. The fields always start from
// the policy as it now stands rather than from an abandoned draft.
function PolicyEditorBody({
  policy,
  titleId,
  canDelete,
  onClose,
  onDelete,
}: Readonly<{
  policy: RetentionPolicy;
  titleId: string;
  canDelete: boolean;
  onClose: () => void;
  onDelete: () => void;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [retainDays, setRetainDays] = useState(String(policy.retain_days));
  const [action, setAction] = useState<RetentionAction>(policy.action);
  const [lawfulBasis, setLawfulBasis] = useState(policy.lawful_basis ?? "");
  const days = parseRetainDays(retainDays);

  const patch = useMutation({
    mutationFn: async ({ body }: PolicyWrite) => {
      return unwrap(
        await api.PATCH("/retention-policies/{id}", {
          params: { path: { id: policy.id } },
          body,
        }),
      );
    },
    onSuccess: (_data, write) => {
      queryClient.invalidateQueries({ queryKey: RETENTION_POLICIES_KEY });
      // The switch leaves the editor open: closing it would drop the fields
      // the operator may be mid-edit on, and focus with them.
      if (write.intent === "save") {
        onClose();
      }
    },
  });

  return (
    <>
      {/* The scope names which policy is open: the dialog covers its row. */}
      <Heading size="large" id={titleId} className="t-h2 modal-title">
        {t(scopeLabelKey(policy.scope))}
      </Heading>
      <div className="form-stack">
        <Field
          label={t("retention.window")}
          hint={
            days === null && retainDays.trim() !== ""
              ? t("retention.windowInvalid")
              : undefined
          }
        >
          {(control) => (
            <TextInput
              {...control}
              inputMode="numeric"
              value={retainDays}
              onChange={(event) => setRetainDays(event.target.value)}
            />
          )}
        </Field>
        <Field label={t("retention.action")}>
          {(control) => (
            <Select
              {...control}
              options={RETENTION_ACTIONS.map((value) => ({
                value,
                label: t(actionLabelKey(value)),
              }))}
              value={action}
              onChange={(value) => {
                const picked = RETENTION_ACTIONS.find(
                  (candidate) => candidate === value,
                );
                if (picked) {
                  setAction(picked);
                }
              }}
            />
          )}
        </Field>
        <Field label={t("retention.lawfulBasis")}>
          {(control) => (
            <TextInput
              {...control}
              value={lawfulBasis}
              onChange={(event) => setLawfulBasis(event.target.value)}
            />
          )}
        </Field>
        {/* Flipping it is the pause, with no Save after it. */}
        <Switch
          label={t("retention.enabled")}
          checked={policy.enabled}
          pending={patch.isPending && patch.variables?.intent === "switch"}
          onChange={(next) =>
            patch.mutate({ intent: "switch", body: { enabled: next } })
          }
        />
        <ErrorLine error={patch.error} />
      </div>
      <div className="actions">
        {/* The confirm replaces this dialog: two dialogs trap focus in the
            wrong one and share one Escape key. */}
        {canDelete && (
          <span className="actions-lead">
            <Button variant="danger" onClick={onDelete}>
              {t("retention.delete")}
            </Button>
          </span>
        )}
        <span className="actions-pair">
          <Button onClick={onClose}>{t("deals.cancel")}</Button>
          <Button
            variant="primary"
            disabled={days === null || patch.isPending}
            onClick={() =>
              days !== null &&
              patch.mutate({
                intent: "save",
                body: {
                  retain_days: days,
                  action,
                  lawful_basis: lawfulBasis.trim() || null,
                },
              })
            }
          >
            {t("retention.save")}
          </Button>
        </span>
      </div>
    </>
  );
}

export type EditorState = Readonly<{
  id: string;
  open: boolean;
  session: number;
}>;

export function PolicyEditor({
  editor,
  policies,
  canDelete,
  onClose,
  onDelete,
}: Readonly<{
  editor: EditorState | null;
  policies: readonly RetentionPolicy[];
  canDelete: boolean;
  onClose: () => void;
  onDelete: (policy: RetentionPolicy) => void;
}>) {
  const titleId = useId();
  // Read from the live list, so the Enabled switch shows what the server holds.
  const policy = policies.find((each) => each.id === editor?.id) ?? null;
  return (
    <Modal
      open={editor?.open === true && policy !== null}
      onClose={onClose}
      labelledBy={titleId}
      intent="form"
    >
      {policy && editor && (
        <PolicyEditorBody
          key={editor.session}
          policy={policy}
          titleId={titleId}
          canDelete={canDelete}
          onClose={onClose}
          onDelete={() => onDelete(policy)}
        />
      )}
    </Modal>
  );
}

// Deleting a policy is not the way to pause one: it drops the rule entirely,
// so records in that scope stop ageing out. The body says so and points at the
// Enabled switch, because that is what the operator usually meant.
export function DeletePolicyModal({
  policy,
  onClose,
}: Readonly<{ policy: RetentionPolicy | null; onClose: () => void }>) {
  const t = useT();
  const queryClient = useQueryClient();

  const remove = useMutation({
    mutationFn: async (target: RetentionPolicy) => {
      unwrap(
        await api.DELETE("/retention-policies/{id}", {
          params: { path: { id: target.id } },
        }),
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: RETENTION_POLICIES_KEY });
      onClose();
    },
  });

  function close() {
    onClose();
    remove.reset();
  }

  return (
    <ConfirmModal
      open={policy !== null}
      onClose={close}
      title={t("retention.deleteTitle")}
      confirmLabel={t("retention.delete")}
      confirmVariant="danger"
      onConfirm={() => policy && remove.mutate(policy)}
      pending={remove.isPending}
      error={remove.isError ? problemMessageOf(remove.error, t) : null}
    >
      <p>
        {policy
          ? t("retention.deleteBody", { scope: t(scopeLabelKey(policy.scope)) })
          : ""}
      </p>
    </ConfirmModal>
  );
}
