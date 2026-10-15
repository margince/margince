import { Info, KeyRound, Lock, Plus, Trash2 } from "lucide-react";
import { useId } from "react";
import { Button, TextInput } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { IconAction } from "../design-system/iconaction";
import { Popover } from "../design-system/popover";
import { useT } from "../i18n";

// One header row as the drawer edits it. `stored` marks a name the server
// already holds a sealed value for; a null `value` keeps that value.
export type HeaderRow = Readonly<{
  key: string;
  name: string;
  value: string | null;
  stored: boolean;
}>;

export type HeaderInput = Readonly<{ name: string; value?: string }>;

let nextRowKey = 0;
function rowKey(): string {
  nextRowKey += 1;
  return `header-${nextRowKey}`;
}

export function headerRowsFrom(names: readonly string[]): HeaderRow[] {
  return names.map((name) => ({
    key: rowKey(),
    name,
    value: null,
    stored: true,
  }));
}

// The PATCH and POST shape: a row with no new value names only its header,
// which the server reads as "keep the stored value".
// A stored row whose Replace was opened but left empty still keeps its value.
function keepsStoredValue(row: HeaderRow): boolean {
  return row.stored && (row.value === null || row.value === "");
}

export function headerInputsOf(rows: readonly HeaderRow[]): HeaderInput[] {
  return rows
    .filter((row) => row.name.trim() !== "")
    .map((row) =>
      keepsStoredValue(row) || row.value === null
        ? { name: row.name.trim() }
        : { name: row.name.trim(), value: row.value },
    );
}

// Whether the rows say anything the stored set does not. An edit that leaves
// the headers alone sends none, and the server keeps them as they are.
export function headersChanged(
  rows: readonly HeaderRow[],
  storedNames: readonly string[],
): boolean {
  const named = rows.filter((row) => row.name.trim() !== "");
  return (
    named.length !== storedNames.length ||
    named.some(
      (row, i) => !keepsStoredValue(row) || row.name !== storedNames[i],
    )
  );
}

export function WebhookHeaderRows({
  rows,
  onChange,
  error,
}: Readonly<{
  rows: readonly HeaderRow[];
  onChange: (rows: HeaderRow[]) => void;
  error?: string;
}>) {
  const t = useT();
  const titleId = useId();
  const update = (key: string, patch: Partial<HeaderRow>) =>
    onChange(rows.map((row) => (row.key === key ? { ...row, ...patch } : row)));
  const add = (name: string, value: string) =>
    onChange([...rows, { key: rowKey(), name, value, stored: false }]);
  const hasAuthorization = rows.some(
    (row) => row.name.trim().toLowerCase() === "authorization",
  );
  return (
    <section className="webhook-section" aria-labelledby={titleId}>
      <div className="webhook-section-head">
        <span className="webhook-section-title">
          <Heading size="small" id={titleId}>
            {t("webhooks.headers.title")}
          </Heading>
          <Popover
            className="webhook-help"
            label={
              <>
                <Info aria-hidden />
                <span className="sr-only">
                  {t("webhooks.headers.helpLabel")}
                </span>
              </>
            }
          >
            <p>{t("webhooks.headers.hint")}</p>
            <p>{t("webhooks.headers.help")}</p>
          </Popover>
        </span>
        <span className="webhook-section-actions">
          {!hasAuthorization && (
            <Button onClick={() => add("Authorization", "Bearer ")}>
              <KeyRound aria-hidden /> {t("webhooks.headers.addAuthorization")}
            </Button>
          )}
          <Button onClick={() => add("", "")}>
            <Plus aria-hidden /> {t("webhooks.headers.add")}
          </Button>
        </span>
      </div>
      {rows.length === 0 ? (
        <p className="t-caption">{t("webhooks.headers.none")}</p>
      ) : (
        <ul className="webhook-header-rows">
          {rows.map((row) => (
            <li key={row.key} className="webhook-header-row">
              <TextInput
                aria-label={t("webhooks.headers.name")}
                value={row.name}
                readOnly={row.stored}
                placeholder={t("webhooks.headers.namePlaceholder")}
                onChange={(event) =>
                  update(row.key, { name: event.target.value })
                }
              />
              {row.value === null ? (
                <span className="webhook-sealed">
                  <Lock aria-hidden />
                  <span>{t("webhooks.headers.valueSet")}</span>
                  <span aria-hidden>
                    ••••••••
                  </span>
                  <Button onClick={() => update(row.key, { value: "" })}>
                    {t("webhooks.headers.replace")}
                  </Button>
                </span>
              ) : (
                <TextInput
                  aria-label={t("webhooks.headers.value")}
                  type="password"
                  autoComplete="off"
                  value={row.value}
                  onChange={(event) =>
                    update(row.key, { value: event.target.value })
                  }
                />
              )}
              <IconAction
                label={t("webhooks.headers.remove", {
                  name: row.name || t("webhooks.headers.name"),
                })}
                icon={<Trash2 aria-hidden />}
                onClick={() => onChange(rows.filter((r) => r.key !== row.key))}
              />
            </li>
          ))}
        </ul>
      )}
      {error && <ErrorLine>{error}</ErrorLine>}
    </section>
  );
}
