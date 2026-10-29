import { Info } from "lucide-react";
import { useState } from "react";
import { Button, Checkbox, SearchField } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Popover } from "../../design-system/popover";
import { SetPickerFrame } from "../../design-system/setpicker";
import { holdsEveryWord, searchWords } from "../../format/collate";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import {
  eventGroupLabelKey,
  eventLabelKey,
  eventWhenKey,
  WEBHOOK_EVENT_GROUPS,
} from "./webhookevents";
import { useEventSampleData } from "./webhookpreview";

const TOTAL_EVENTS = WEBHOOK_EVENT_GROUPS.reduce(
  (sum, group) => sum + group.types.length,
  0,
);

function kindOf(value: unknown): string {
  if (Array.isArray(value)) {
    return "list";
  }
  return value === null ? "null" : typeof value;
}

function EventFields({ type }: Readonly<{ type: string }>) {
  const t = useT();
  const sample = useEventSampleData(type);
  if (sample.isPending) {
    return null;
  }
  if (sample.isError) {
    return <p className="t-caption">{t("webhooks.events.fieldsFailed")}</p>;
  }
  const fields = Object.entries(sample.data);
  if (fields.length === 0) {
    return <p className="t-caption">{t("webhooks.events.noFields")}</p>;
  }
  return (
    <ul className="webhook-event-fields">
      {fields.map(([name, value]) => (
        <li key={name}>
          <code>{name}</code> <span className="t-caption">{kindOf(value)}</span>
        </li>
      ))}
    </ul>
  );
}

function EventInfo({ type }: Readonly<{ type: string }>) {
  const t = useT();
  const label = t(eventLabelKey(type));
  return (
    <Popover
      className="webhook-help"
      label={
        <>
          <Info aria-hidden />
          <span className="sr-only">
            {t("webhooks.events.info", { event: label })}
          </span>
        </>
      }
    >
      <p className="t-label">{label}</p>
      <p>
        <code>{type}</code>
      </p>
      <p className="t-label">{t("webhooks.events.when")}</p>
      <p>{t(eventWhenKey(type))}</p>
      <p className="t-label">{t("webhooks.events.fields")}</p>
      <EventFields type={type} />
    </Popover>
  );
}

function matches(type: string, label: string, group: string, query: string) {
  return holdsEveryWord(searchWords(query), [type, label, group]);
}

function PickerPanel({
  value,
  onChange,
  onDone,
}: Readonly<{
  value: readonly string[];
  onChange: (next: string[]) => void;
  onDone: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const [query, setQuery] = useState("");
  const chosen = new Set(value);
  const toggle = (types: readonly string[], on: boolean) => {
    const next = new Set(chosen);
    for (const type of types) {
      if (on) {
        next.add(type);
      } else {
        next.delete(type);
      }
    }
    onChange([...next]);
  };
  const groups = WEBHOOK_EVENT_GROUPS.map((group) => {
    const groupLabel = t(eventGroupLabelKey(group.key));
    return {
      key: group.key,
      label: groupLabel,
      all: group.types,
      types: group.types.filter((type) =>
        matches(type, t(eventLabelKey(type)), groupLabel, query),
      ),
    };
  }).filter((group) => group.types.length > 0);
  return (
    <div className="webhook-picker">
      <SearchField
        aria-label={t("webhooks.events.search")}
        placeholder={t("webhooks.events.search")}
        value={query}
        onChange={(event) => setQuery(event.target.value)}
      />
      <div className="webhook-picker-count">
        <span className="t-caption">
          {plural("webhooks.events.selected", value.length, {
            count: formatNumber(value.length, locale),
            total: formatNumber(TOTAL_EVENTS, locale),
          })}
        </span>
        {value.length > 0 && (
          <Button variant="link" onClick={() => onChange([])}>
            {t("webhooks.events.clear")}
          </Button>
        )}
      </div>
      <div className="webhook-picker-list">
        {groups.length === 0 && (
          <p className="t-caption">{t("webhooks.events.noMatch")}</p>
        )}
        {groups.map((group) => {
          const allOn = group.types.every((type) => chosen.has(type));
          return (
            <section key={group.key} aria-label={group.label}>
              <div className="webhook-picker-group">
                <span className="webhook-picker-group-name">
                  <span className="t-label">{group.label}</span>
                  <span className="t-caption">
                    {plural(
                      "webhooks.events.groupCount",
                      group.all.filter((type) => chosen.has(type)).length,
                      {
                        count: formatNumber(
                          group.all.filter((type) => chosen.has(type)).length,
                          locale,
                        ),
                        total: formatNumber(group.all.length, locale),
                      },
                    )}
                  </span>
                </span>
                <Button
                  variant="link"
                  onClick={() => toggle(group.types, !allOn)}
                >
                  {allOn
                    ? t("webhooks.events.clear")
                    : t("webhooks.events.selectAll")}
                </Button>
              </div>
              <ul>
                {group.types.map((type) => (
                  <li key={type} className="webhook-picker-row">
                    <Checkbox
                      checked={chosen.has(type)}
                      onChange={(event) => toggle([type], event.target.checked)}
                      label={
                        <span className="webhook-picker-label">
                          <span>
                            <span className="webhook-picker-name">
                              {t(eventLabelKey(type))}
                            </span>{" "}
                            <code>{type}</code>
                          </span>
                          <span className="t-caption">
                            {t(eventWhenKey(type))}
                          </span>
                        </span>
                      }
                    />
                    <EventInfo type={type} />
                  </li>
                ))}
              </ul>
            </section>
          );
        })}
      </div>
      <div className="webhook-picker-foot">
        <span className="t-caption">{t("webhooks.events.appliesAtOnce")}</span>
        <Button variant="primary" onClick={onDone}>
          {t("webhooks.events.done")}
        </Button>
      </div>
    </div>
  );
}

// The chosen events as tags, and the whole catalog behind "Add events", so a
// long list never stretches the drawer.
export function WebhookEventPicker({
  value,
  onChange,
  error,
}: Readonly<{
  value: readonly string[];
  onChange: (next: string[]) => void;
  error?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const [open, setOpen] = useState(false);
  return (
    <div className="webhook-events">
      <div className="webhook-events-head">
        <span className="webhook-section-title">
          <span className="t-label">{t("webhooks.field.eventTypes")} *</span>
          <Popover
            className="webhook-help"
            label={
              <>
                <Info aria-hidden />
                <span className="sr-only">
                  {t("webhooks.events.helpLabel")}
                </span>
              </>
            }
          >
            <p>{t("webhooks.events.help")}</p>
          </Popover>
        </span>
        {value.length > 0 && (
          <span className="webhook-events-count">
            <span className="t-caption">
              {plural("webhooks.events.count", value.length, {
                count: formatNumber(value.length, locale),
              })}
            </span>
            <Button variant="link" onClick={() => onChange([])}>
              {t("webhooks.events.clear")}
            </Button>
          </span>
        )}
      </div>
      <SetPickerFrame
        chosen={value.map((type) => ({
          id: type,
          label: t(eventLabelKey(type)),
        }))}
        removeLabel={(item) =>
          t("webhooks.events.remove", { event: item.label })
        }
        onRemove={(type) => onChange(value.filter((v) => v !== type))}
        none={t("webhooks.events.none")}
        addLabel={t("webhooks.events.add")}
        invalid={!!error}
        wide
        open={open}
        onOpenChange={setOpen}
      >
        <PickerPanel
          value={value}
          onChange={onChange}
          onDone={() => setOpen(false)}
        />
      </SetPickerFrame>
      {error && <ErrorLine>{error}</ErrorLine>}
      <p className="t-caption">{t("webhooks.events.visibility")}</p>
    </div>
  );
}
