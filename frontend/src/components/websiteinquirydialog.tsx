// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { Badge, Button, Field, TextInput } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { QueryStates } from "../design-system/querystates";
import { RecordFormDialog } from "../design-system/recordformdialog";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { useCan } from "../lib/capability";
import { unwrap } from "../lib/problem";
import { useMe } from "../lib/session";
import {
  newInquiryPolicy,
  WEB_FORMS_KEY,
  type WebsiteInquiryPolicy,
} from "../lib/websiteinquiries";
import { EntityRef } from "./entityref";
import { useUnsavedGuard } from "./unsaved";
import { inquiryModeLabel, WebsiteInquiryFields } from "./websiteinquiryfields";

type WebForm = components["schemas"]["WebForm"];
type Automation = components["schemas"]["Automation"];

export function WebsiteInquiryDialog({
  form,
  formId,
  automation,
  onClose,
}: Readonly<{
  form?: WebForm;
  formId?: string;
  automation?: Automation;
  onClose: () => void;
}>) {
  const t = useT();
  const canRead = useCan("integrations", "read");
  const [chosen, setChosen] = useState(form);
  const forms = useQuery({
    queryKey: [WEB_FORMS_KEY],
    enabled: canRead && !chosen && !formId,
    queryFn: async () => unwrap(await api.GET("/web-forms")),
  });
  const single = useQuery({
    queryKey: ["website-inquiry-form", formId],
    gcTime: 0,
    enabled: canRead && !chosen && !!formId,
    queryFn: async () =>
      unwrap(
        await api.GET("/web-forms/{id}", {
          params: { path: { id: formId ?? "" } },
        }),
      ),
  });
  const selected = chosen ?? single.data;
  const policyId = automation?.id ?? selected?.handling_automation_id;
  const policy = useQuery({
    queryKey: ["website-inquiry-policy", policyId],
    gcTime: 0,
    enabled: !!policyId,
    queryFn: async () =>
      unwrap(
        await api.GET("/automations/{id}", {
          params: { path: { id: policyId ?? "" } },
        }),
      ),
  });
  if (selected && (!policyId || policy.data)) {
    return (
      <InquiryPolicyEditor
        key={selected.id}
        form={selected}
        automation={policy.data}
        onClose={onClose}
      />
    );
  }
  const query = selected ? policy : formId ? single : forms;
  return (
    <RecordFormDialog
      open
      title={t("inquiry.handling")}
      onClose={onClose}
      fieldCount={1}
      actions={<Button onClick={onClose}>{t("deals.cancel")}</Button>}
      form={
        canRead ? (
          <QueryStates query={query} pendingLabel={t("inquiry.handling")}>
            <Field label={t("inquiry.form")}>
              {(control) => (
                <Select
                  {...control}
                  options={(forms.data?.data ?? []).map((entry) => ({
                    value: entry.id,
                    label: entry.name,
                  }))}
                  value=""
                  onChange={(id) =>
                    setChosen(forms.data?.data.find((entry) => entry.id === id))
                  }
                  placeholder={t("inquiry.chooseForm")}
                />
              )}
            </Field>
          </QueryStates>
        ) : (
          <p>{t("inquiry.formAccess")}</p>
        )
      }
    />
  );
}

type HandlingSave = Readonly<{
  form: WebForm;
  automation?: Automation;
  name: string;
  policy: WebsiteInquiryPolicy;
  enabled: boolean;
}>;

function InquiryPolicyEditor({
  form,
  automation,
  onClose,
}: Readonly<{ form: WebForm; automation?: Automation; onClose: () => void }>) {
  const t = useT();
  const id = useId();
  const client = useQueryClient();
  const me = useMe();
  const [opening] = useState(() => ({ form, automation }));
  const [initial] = useState(
    () =>
      automation?.inquiry_policy ??
      newInquiryPolicy(t("inquiry.taskTitleDefault"), form.id),
  );
  const [policy, setPolicy] = useState(initial);
  const [initialName] = useState(
    () => automation?.name ?? `${t("inquiry.handling")} · ${form.name}`,
  );
  const [name, setName] = useState(initialName);
  const [reviewing, setReviewing] = useState(false);
  const save = useMutation({
    mutationFn: async (review: HandlingSave) =>
      unwrap(
        await api.PUT("/web-forms/{id}/handling", {
          params: {
            path: { id: review.form.id },
            ...ifMatch(review.form.version),
          },
          body: {
            name: review.name,
            policy: review.policy,
            enabled: review.enabled,
            ...(review.automation
              ? {
                  automation_id: review.automation.id,
                  automation_version: requireVersion(review.automation.version),
                }
              : {}),
          },
        }),
      ),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: [WEB_FORMS_KEY] });
      await client.invalidateQueries({ queryKey: ["automations"] });
      onClose();
    },
  });
  const saveReview = (enabled: boolean) =>
    save.mutate({
      form: opening.form,
      automation: opening.automation,
      name: name.trim(),
      policy,
      enabled,
    });
  const ready =
    [
      policy.form_id,
      policy.coordinator_id,
      policy.timezone,
      policy.task_title,
    ].every((value) => value.trim() !== "") &&
    (policy.mode === "coordinator" || (policy.routing.owners?.length ?? 0) > 0);
  const dirty =
    name !== initialName || JSON.stringify(policy) !== JSON.stringify(initial);
  useUnsavedGuard(dirty);
  return (
    <RecordFormDialog
      open
      title={t("inquiry.handling")}
      onClose={onClose}
      fieldCount={8}
      unsaved={dirty}
      closeDisabled={save.isPending}
      form={
        <form
          id={id}
          className="form-stack"
          onSubmit={(event) => {
            event.preventDefault();
            setReviewing(true);
          }}
        >
          <Badge
            tone={
              opening.automation?.status === "enabled" ? "success" : "warning"
            }
          >
            {t(
              opening.automation?.status === "enabled"
                ? "auto.statusEnabled"
                : opening.automation
                  ? "auto.statusPaused"
                  : "inquiry.needsSetup",
            )}
          </Badge>
          {reviewing ? (
            <InquiryPolicyReview
              policy={policy}
              form={opening.form}
              name={name}
              authorizer={opening.automation?.owner_id ?? me.data?.user.id}
            />
          ) : (
            <>
              <Field label={t("auto.name")}>
                {(control) => (
                  <TextInput
                    {...control}
                    required
                    maxLength={200}
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                  />
                )}
              </Field>
              <WebsiteInquiryFields
                form={opening.form}
                value={policy}
                onChange={setPolicy}
              />
            </>
          )}
          <ErrorLine error={save.error} />
        </form>
      }
      actions={
        <>
          <Button
            disabled={save.isPending}
            onClick={reviewing ? () => setReviewing(false) : onClose}
          >
            {t(reviewing ? "inquiry.back" : "deals.cancel")}
          </Button>
          {reviewing ? (
            <>
              <Button
                disabled={save.isPending}
                onClick={() => saveReview(false)}
              >
                {t("inquiry.savePaused")}
              </Button>
              <Button
                variant="primary"
                pending={save.isPending}
                onClick={() => saveReview(true)}
              >
                {t("inquiry.enable")}
              </Button>
            </>
          ) : (
            <Button type="submit" form={id} variant="primary" disabled={!ready}>
              {t("inquiry.review")}
            </Button>
          )}
        </>
      }
    />
  );
}

function InquiryPolicyReview({
  policy,
  form,
  name,
  authorizer,
}: Readonly<{
  policy: WebsiteInquiryPolicy;
  form: WebForm;
  name: string;
  authorizer?: string;
}>) {
  const t = useT();
  return (
    <>
      <p>{t("inquiry.conversion")}</p>
      <dl>
        <dt>{t("auto.name")}</dt>
        <dd>{name}</dd>
        <dt>{t("auto.colOwner")}</dt>
        <dd>
          {authorizer ? (
            <EntityRef kind="user" id={authorizer} asText />
          ) : (
            t("auto.ownerNone")
          )}
        </dd>
        <dt>{t("inquiry.form")}</dt>
        <dd>{form.name}</dd>
        <dt>{t("inquiry.mode")}</dt>
        <dd>{inquiryModeLabel(policy.mode, t)}</dd>
        {policy.mode !== "coordinator" && (
          <>
            <dt>{t("inquiry.colleagues")}</dt>
            <dd>
              <ul>
                {(policy.routing.owners ?? []).map((owner) => (
                  <li key={owner}>
                    <EntityRef kind="user" id={owner} asText />
                  </li>
                ))}
              </ul>
            </dd>
          </>
        )}
        <dt>{t("inquiry.coordinator")}</dt>
        <dd>
          <EntityRef kind="user" id={policy.coordinator_id} asText />
        </dd>
        <dt>{t("inquiry.taskTitle")}</dt>
        <dd>{policy.task_title}</dd>
        <dt>{t("inquiry.dueDays")}</dt>
        <dd>{policy.due_in_days}</dd>
        <dt>{t("auto.deadlineTimezone")}</dt>
        <dd>{policy.timezone}</dd>
      </dl>
      <p>{t("inquiry.futureOnly")}</p>
    </>
  );
}
