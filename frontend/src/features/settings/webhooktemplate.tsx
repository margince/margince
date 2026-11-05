import { Braces, ChevronDown } from "lucide-react";
import { useState } from "react";
import { Button, Field, SegmentedControl } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { JsonField } from "../../design-system/jsonfield";
import { Popover } from "../../design-system/popover";
import { Select } from "../../design-system/select";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { problemFieldErrorsOf, problemMessageOf } from "../../lib/problem";
import { eventLabelKey } from "./webhookevents";
import {
  ENVELOPE_VARIABLES,
  WEBHOOK_BODY_PRESETS,
  type WebhookBodyPreset,
} from "./webhookpresets";
import {
  useBodyPreview,
  useEventSampleData,
  type WebhookBodyContentType,
} from "./webhookpreview";

type Tab = "template" | "preview";

// The placeholder text a variable inserts: a JSON body takes text inside
// quotes, so a variable is quoted unless it is the whole data object.
function placeholderFor(path: string, contentType: WebhookBodyContentType) {
  if (path === "data") {
    return "{{ data | json }}";
  }
  return contentType === "application/json"
    ? `"{{ ${path} }}"`
    : `{{ ${path} }}`;
}

// The server names a refused placeholder by "line N, column M"; the editor
// marks that line.
function problemLine(error: string | undefined): number[] {
  const line = error?.match(/line (\d+), column/)?.[1];
  return line ? [Number(line)] : [];
}

function TemplatePreview({
  eventType,
  template,
  contentType,
}: Readonly<{
  eventType: string;
  template: string;
  contentType: WebhookBodyContentType;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const preview = useBodyPreview(eventType, template, contentType);
  if (eventType === "") {
    return <p className="t-caption">{t("webhooks.body.previewEmpty")}</p>;
  }
  if (preview.isError) {
    const fieldError = problemFieldErrorsOf(preview.error)[0];
    return (
      <ErrorLine>
        {fieldError?.message ?? problemMessageOf(preview.error, t)}
      </ErrorLine>
    );
  }
  if (!preview.data) {
    return null;
  }
  return (
    <div className="webhook-preview">
      <p className="t-caption">
        {t("webhooks.body.size", {
          size: formatNumber(preview.data.byte_length, locale),
        })}
      </p>
      <pre
        className="code-block webhook-code"
        data-testid="webhook-preview-body"
      >
        {preview.data.body}
      </pre>
    </div>
  );
}

export function WebhookTemplateEditor({
  template,
  contentType,
  eventTypes,
  error,
  onChange,
}: Readonly<{
  template: string;
  contentType: WebhookBodyContentType;
  eventTypes: readonly string[];
  error?: string;
  onChange: (next: {
    template: string;
    contentType: WebhookBodyContentType;
  }) => void;
}>) {
  const t = useT();
  const [tab, setTab] = useState<Tab>("template");
  const [sample, setSample] = useState("");
  const [preset, setPreset] = useState("");
  const [inserting, setInserting] = useState(false);
  const sampleType = eventTypes.includes(sample)
    ? sample
    : (eventTypes[0] ?? "");
  const sampleData = useEventSampleData(sampleType);
  const [caret, setCaret] = useState<{ start: number; end: number } | null>(
    null,
  );
  const variables = [
    ...ENVELOPE_VARIABLES,
    "data",
    ...Object.keys(sampleData.data ?? {}).map((field) => `data.${field}`),
  ];
  const insert = (path: string) => {
    const text = placeholderFor(path, contentType);
    const at = caret?.start ?? template.length;
    const end = caret?.end ?? template.length;
    setCaret({ start: at + text.length, end: at + text.length });
    onChange({
      template: template.slice(0, at) + text + template.slice(end),
      contentType,
    });
  };
  return (
    <div className="webhook-template">
      <div className="form-row">
        <Field label={t("webhooks.body.startFrom")}>
          {(field) => (
            <Select
              {...field}
              value={preset}
              placeholder={t("webhooks.body.startFromPlaceholder")}
              options={(
                Object.keys(WEBHOOK_BODY_PRESETS) as WebhookBodyPreset[]
              ).map((key) => ({
                value: key,
                label: t(`webhooks.body.preset.${key}`),
              }))}
              onChange={(picked) => {
                setPreset(picked);
                const chosen =
                  WEBHOOK_BODY_PRESETS[picked as WebhookBodyPreset];
                onChange({
                  template: chosen.template,
                  contentType: chosen.contentType,
                });
              }}
            />
          )}
        </Field>
        <Field label={t("webhooks.body.sample")}>
          {(field) => (
            <Select
              {...field}
              value={sampleType}
              placeholder={t("webhooks.body.previewEmpty")}
              options={eventTypes.map((type) => ({
                value: type,
                label: t(eventLabelKey(type)),
              }))}
              onChange={setSample}
            />
          )}
        </Field>
      </div>
      <div className="webhook-template-tabs">
        <SegmentedControl
          label={t("webhooks.body.template")}
          options={["template", "preview"] as const}
          value={tab}
          onChange={setTab}
          labels={{
            template: t("webhooks.body.tab.template"),
            preview: t("webhooks.body.tab.preview"),
          }}
        />
        {tab === "template" && (
          <Popover
            dialog
            variant="ghost"
            open={inserting}
            onOpenChange={setInserting}
            panelClassName="webhook-insert-panel"
            label={
              <>
                <Braces aria-hidden /> {t("webhooks.body.insert")}
                <ChevronDown aria-hidden />
              </>
            }
          >
            <ul className="webhook-insert-list">
              {variables.map((path) => (
                <li key={path}>
                  <Button
                    variant="link"
                    onClick={() => {
                      insert(path);
                      setInserting(false);
                    }}
                  >
                    <code>{path}</code>
                  </Button>
                </li>
              ))}
            </ul>
          </Popover>
        )}
      </div>
      {tab === "template" ? (
        <Field
          label={t("webhooks.body.template")}
          labelHidden
          hint={t("webhooks.body.templateHint")}
          error={error}
        >
          {(field) => (
            <JsonField
              {...field}
              rows={10}
              value={template}
              invalid={error !== undefined}
              problemLines={problemLine(error)}
              onSelect={(event) =>
                setCaret({
                  start: event.currentTarget.selectionStart,
                  end: event.currentTarget.selectionEnd,
                })
              }
              onChange={(next) => onChange({ template: next, contentType })}
            />
          )}
        </Field>
      ) : (
        <TemplatePreview
          eventType={sampleType}
          template={template}
          contentType={contentType}
        />
      )}
    </div>
  );
}
