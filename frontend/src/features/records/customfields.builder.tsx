import { X } from "lucide-react";
import { useId, useState } from "react";
import {
  Badge,
  Button,
  Checkbox,
  Field,
  SegmentedControl,
  TextInput,
} from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Select } from "../../design-system/select";
import { useToast } from "../../design-system/toast";
import { AutonomyDot } from "../../design-system/trust";
import { useT } from "../../i18n";
import {
  apiKey,
  CF_TYPES,
  type CfObject,
  type CfType,
  ddlPreview,
  looksStructural,
} from "../../lib/customfields.logic";
import { useRecordKindDirectory } from "../../lib/recordkinds";
import { typeLabels } from "./customfields.labels";
import "./customfields.css";

// The add-field builder (AC-custom-fields-3..5/8) turns a plain label into one
// typed column on an existing object. The fixed API key and the pending DDL show
// before Confirm, and a structural-sounding label is refused up front. The gate
// is a `warning` Callout, how the surface says something about itself elsewhere.

export type NewFieldDraft = {
  object: CfObject;
  label: string;
  type: CfType;
  currency: string;
  options: string[];
  showInCreate: boolean;
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
  // its secondary verb closes that dialog. A form discarded on close has
  // nothing to reset to.
  onCancel: () => void;
}>) {
  const toast = useToast();
  const t = useT();
  const [label, setLabel] = useState("");
  const [type, setType] = useState<CfType>("text");
  const [currency, setCurrency] = useState("EUR");
  const [options, setOptions] = useState<string[]>([""]);
  const [showInCreate, setShowInCreate] = useState(true);
  const showInCreateHintId = useId();
  // A record field points at a kind an installed extension declared, so the
  // type is offered only when there is one to pick.
  const kinds = useRecordKindDirectory().data?.data ?? [];
  const offeredTypes = CF_TYPES.filter(
    (one) => one !== "record" || kinds.length > 0,
  );
  const structural = looksStructural(label);
  // A picklist needs a non-blank option and a currency field a 3-letter
  // ISO-4217 code. Confirm stays disabled until that shape is valid too.
  const typeShapeValid =
    ((type !== "picklist" && type !== "multiselect") ||
      options.some((opt) => opt.trim().length > 0)) &&
    (type !== "record" || kinds.some((kind) => kind.name === options[0])) &&
    (type !== "currency" || /^[A-Za-z]{3}$/.test(currency.trim()));
  const canConfirm =
    !pending && label.trim().length > 0 && !structural && typeShapeValid;

  const setOptionAt = (idx: number, value: string) => {
    setOptions((current) => current.map((opt, i) => (i === idx ? value : opt)));
  };

  const removeOption = (idx: number) => {
    // The last option row is a floor, not a delete target. The refusal shows
    // as a toast instead of being swallowed.
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
    onSubmit({
      object,
      label: label.trim(),
      type,
      currency,
      options,
      showInCreate,
    });
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
            options={offeredTypes}
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

        {type === "record" && (
          <Field label={t("cf.recordKind")}>
            {(control) => (
              <Select
                {...control}
                options={kinds.map((kind) => ({
                  value: kind.name,
                  label: kind.display_name,
                }))}
                value={options[0] ?? ""}
                placeholder={t("cf.recordKindPlaceholder")}
                onChange={(kind) => setOptions([kind])}
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

        <div className="field">
          <Checkbox
            checked={showInCreate}
            onChange={(event) => setShowInCreate(event.target.checked)}
            label={t("cf.showInCreate")}
            aria-describedby={showInCreateHintId}
          />
          <p className="t-caption" id={showInCreateHintId}>
            {t("cf.showInCreateHint")}
          </p>
        </div>

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

      {/* Cancel first, then the verb that writes: the order every dialog in
          this tree uses. Closing the dialog discards the draft, so no reset
          control is needed. */}
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
