import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Info } from "lucide-react";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch } from "../../api/version";
import { useRecordOwnerOptions } from "../../components/assigneepicker";
import { Button, Field, Modal, TextInput } from "../../design-system/atoms";
import {
  DrawerBody,
  DrawerFoot,
  DrawerHead,
} from "../../design-system/drawerbands";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Popover } from "../../design-system/popover";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import { useCan } from "../../lib/capability";
import {
  problemFieldErrorsOf,
  problemMessageOf,
  unwrap,
} from "../../lib/problem";
import { WebhookEventPicker } from "./webhookeventpicker";
import {
  type HeaderRow,
  headerInputsOf,
  headerRowsFrom,
  headersChanged,
  WebhookHeaderRows,
} from "./webhookheaders";
import { WEBHOOK_BODY_CONTENT_TYPES } from "./webhookpresets";
import type { WebhookBodyContentType } from "./webhookpreview";
import { WebhookTemplateEditor } from "./webhooktemplate";
import "./webhookdrawer.css";

type WebhookSubscription = components["schemas"]["WebhookSubscription"];
type WebhookSubscriptionCreated =
  components["schemas"]["WebhookSubscriptionCreated"];
type BodyFormat = "standard" | "custom";

export type WebhookForm = Readonly<{
  targetUrl: string;
  state: WebhookSubscription["state"];
  // The member every delivery runs under; "" before a subscription exists.
  ownerId: string;
  eventTypes: string[];
  headers: HeaderRow[];
  format: BodyFormat;
  template: string;
  contentType: WebhookBodyContentType;
}>;

export function formFor(subscription?: WebhookSubscription): WebhookForm {
  return {
    targetUrl: subscription?.target_url ?? "",
    state: subscription?.state ?? "active",
    ownerId: subscription?.owner_id ?? "",
    eventTypes: [...(subscription?.event_types ?? [])],
    headers: headerRowsFrom(
      (subscription?.custom_headers ?? []).map((h) => h.name),
    ),
    format: subscription?.body_template ? "custom" : "standard",
    template: subscription?.body_template ?? "",
    contentType: subscription?.body_content_type ?? "application/json",
  };
}

// The request body shared by create and update: the body format is sent
// whole, so switching back to the standard envelope clears the template.
export function webhookBodyOf(
  form: WebhookForm,
  storedHeaders?: readonly string[],
) {
  const custom = form.format === "custom";
  const sendHeaders =
    storedHeaders === undefined || headersChanged(form.headers, storedHeaders);
  return {
    event_types: form.eventTypes,
    ...(sendHeaders ? { custom_headers: headerInputsOf(form.headers) } : {}),
    body_template: custom ? form.template : null,
    body_content_type: custom ? form.contentType : null,
  };
}

const FIELDS_WITH_A_SLOT = new Set([
  "target_url",
  "owner_id",
  "event_types",
  "custom_headers",
  "body_template",
  "body_content_type",
]);

type SaveVariables = Readonly<{
  form: WebhookForm;
  subscription?: WebhookSubscription;
}>;

async function saveSubscription({
  form,
  subscription,
}: SaveVariables): Promise<WebhookSubscriptionCreated | WebhookSubscription> {
  if (subscription) {
    return unwrap(
      await api.PATCH("/webhook-subscriptions/{id}", {
        params: {
          path: { id: subscription.id },
          ...ifMatch(subscription.version),
        },
        body: {
          state: form.state,
          // Sent only when changed: re-stating an owner who left is no handover.
          ...(form.ownerId !== subscription.owner_id
            ? { owner_id: form.ownerId }
            : {}),
          ...webhookBodyOf(
            form,
            subscription.custom_headers.map((header) => header.name),
          ),
        },
      }),
    );
  }
  return unwrap(
    await api.POST("/webhook-subscriptions", {
      body: { target_url: form.targetUrl.trim(), ...webhookBodyOf(form) },
    }),
  );
}

// Who the subscription runs under. Deliveries stop while the owner is
// inactive, so the current owner stays choosable by name, marked as gone. The
// colleagues who may own records are offered beside them.
function WebhookOwnerField({
  subscription,
  value,
  error,
  onChange,
}: Readonly<{
  subscription: WebhookSubscription;
  value: string;
  error?: string;
  onChange: (ownerId: string) => void;
}>) {
  const t = useT();
  const { options } = useRecordOwnerOptions();
  const current = options.some(
    (option) => option.value === subscription.owner_id,
  )
    ? []
    : [
        {
          value: subscription.owner_id,
          label:
            subscription.owner_active === false
              ? t("webhooks.field.ownerInactive")
              : t("webhooks.field.ownerCurrent"),
        },
      ];
  return (
    <Field
      label={t("webhooks.field.owner")}
      hint={t("webhooks.field.ownerHint")}
      error={error}
    >
      {(field) => (
        <Select
          {...field}
          value={value}
          options={[...current, ...options]}
          onChange={onChange}
        />
      )}
    </Field>
  );
}

function HelpPopover({
  label,
  text,
}: Readonly<{ label: string; text: string }>) {
  return (
    <Popover
      className="webhook-help"
      label={
        <>
          <Info aria-hidden />
          <span className="sr-only">{label}</span>
        </>
      }
    >
      <p>{text}</p>
    </Popover>
  );
}

// Create and edit in one wide drawer: the form holds an editor and a preview,
// which a short dialog cannot.
export function WebhookSubscriptionDrawer({
  subscription,
  onClose,
  onCreated,
}: Readonly<{
  subscription?: WebhookSubscription;
  onClose: () => void;
  onCreated?: (secret: string) => void;
}>) {
  const t = useT();
  const titleId = useId();
  const queryClient = useQueryClient();
  const [form, setForm] = useState<WebhookForm>(() => formFor(subscription));
  const set = (patch: Partial<WebhookForm>) =>
    setForm((prev) => ({ ...prev, ...patch }));
  const save = useMutation({
    mutationFn: saveSubscription,
    onSuccess: (saved) => {
      queryClient.invalidateQueries({ queryKey: ["webhook-subscriptions"] });
      if ("signing_secret" in saved) {
        onCreated?.(saved.signing_secret);
      }
      onClose();
    },
  });
  const fieldErrors = new Map(
    problemFieldErrorsOf(save.error).map((problem) => [
      problem.field,
      problem.message,
    ]),
  );
  const fieldError = (name: string) => fieldErrors.get(name);
  // A refusal on a field the drawer has no slot for is said once, at the foot.
  const unplaced = [...fieldErrors.entries()].filter(
    ([field]) => !FIELDS_WITH_A_SLOT.has(field),
  );
  const generalError = !save.isError
    ? null
    : fieldErrors.size === 0
      ? problemMessageOf(save.error, t)
      : (unplaced[0]?.[1] ?? null);
  const editing = subscription !== undefined;
  // Naming who a webhook runs as is acting for that member, which only a
  // member administrator may do; the server refuses everyone else.
  const actsForMembers = useCan("user_admin", "update");
  const eventsMissing = form.eventTypes.length === 0;
  return (
    <Modal
      open
      onClose={onClose}
      labelledBy={titleId}
      intent="drawer-reading"
      closeDisabled={save.isPending}
    >
      <DrawerHead>
        <Heading size="large" id={titleId} className="t-h2 modal-title">
          {editing ? t("webhooks.drawer.editTitle") : t("webhooks.new")}
        </Heading>
        <p>{t("webhooks.drawer.sub")}</p>
      </DrawerHead>
      <DrawerBody className="form-stack webhook-drawer">
        <div className="form-row webhook-target-row">
          <Field
            label={t("webhooks.field.targetUrl")}
            required={!editing}
            hint={
              editing
                ? t("webhooks.field.targetUrlFixed")
                : t("webhooks.field.targetUrlHint")
            }
            error={fieldError("target_url")}
          >
            {(field) => (
              <TextInput
                {...field}
                type="url"
                value={form.targetUrl}
                readOnly={editing}
                placeholder={t("webhooks.field.targetUrlPlaceholder")}
                onChange={(event) => set({ targetUrl: event.target.value })}
              />
            )}
          </Field>
          {editing && (
            <Field
              label={t("webhooks.field.state")}
              hint={t("webhooks.field.stateHint")}
            >
              {(field) => (
                <Select
                  {...field}
                  value={form.state}
                  options={[
                    { value: "active", label: t("webhooks.state.active") },
                    { value: "paused", label: t("webhooks.state.paused") },
                  ]}
                  onChange={(state) =>
                    set({ state: state as WebhookForm["state"] })
                  }
                />
              )}
            </Field>
          )}
          {subscription && actsForMembers && (
            <WebhookOwnerField
              subscription={subscription}
              value={form.ownerId}
              error={fieldError("owner_id")}
              onChange={(ownerId) => set({ ownerId })}
            />
          )}
        </div>
        <div className="field">
          <WebhookEventPicker
            value={form.eventTypes}
            onChange={(eventTypes) => set({ eventTypes })}
            error={fieldError("event_types")}
          />
        </div>
        <WebhookHeaderRows
          rows={form.headers}
          onChange={(headers) => set({ headers })}
          error={fieldError("custom_headers")}
        />
        <div className="form-row webhook-body-row">
          <Field
            label={t("webhooks.body.format")}
            labelEnd={
              <HelpPopover
                label={t("webhooks.body.helpLabel")}
                text={t("webhooks.body.help")}
              />
            }
            hint={
              form.format === "custom"
                ? t("webhooks.body.customHint")
                : t("webhooks.body.standardHint")
            }
          >
            {(field) => (
              <Select
                {...field}
                value={form.format}
                options={[
                  { value: "standard", label: t("webhooks.body.standard") },
                  { value: "custom", label: t("webhooks.body.custom") },
                ]}
                onChange={(format) => set({ format: format as BodyFormat })}
              />
            )}
          </Field>
          {form.format === "custom" && (
            <Field
              label={t("webhooks.body.contentType")}
              hint={t("webhooks.body.contentTypeHint")}
              error={fieldError("body_content_type")}
            >
              {(field) => (
                <Select
                  {...field}
                  value={form.contentType}
                  options={WEBHOOK_BODY_CONTENT_TYPES.map((ct) => ({
                    value: ct,
                    label: ct,
                  }))}
                  onChange={(ct) =>
                    set({ contentType: ct as WebhookBodyContentType })
                  }
                />
              )}
            </Field>
          )}
        </div>
        {form.format === "custom" && (
          <WebhookTemplateEditor
            template={form.template}
            contentType={form.contentType}
            eventTypes={form.eventTypes}
            error={fieldError("body_template")}
            onChange={(next) => set(next)}
          />
        )}
        {generalError && <ErrorLine>{generalError}</ErrorLine>}
      </DrawerBody>
      <DrawerFoot className="actions">
        <Button onClick={onClose} disabled={save.isPending}>
          {t("webhooks.drawer.cancel")}
        </Button>
        <Button
          variant="primary"
          data-testid="save-webhook-subscription"
          pending={save.isPending}
          reason={eventsMissing ? t("webhooks.events.required") : undefined}
          onClick={() => save.mutate({ form, subscription })}
        >
          {editing
            ? t("webhooks.drawer.saveSubscription")
            : t("webhooks.drawer.create")}
        </Button>
      </DrawerFoot>
    </Modal>
  );
}
