import { useId, useState } from "react";
import {
  Badge,
  Button,
  Checkbox,
  Field,
  TextInput,
} from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ErrorLine } from "../../design-system/errorline";
import { useFieldFaults } from "../../design-system/savepage";
import { Select } from "../../design-system/select";
import { AutonomyDot } from "../../design-system/trust";
import { useLocale, useT } from "../../i18n";
import {
  apiKey,
  CF_TYPES,
  type CfObject,
  type CfType,
  looksStructural,
} from "../../lib/customfields.logic";
import { fieldCheckSentence, refusalSentences } from "../../lib/problem";
import { useRecordKindDirectory } from "../../lib/recordkinds";
import { ActivityKindsSelect } from "./customfields.kinds";
import { typeLabels } from "./customfields.labels";
import { OptionRows } from "./customfields.optionrows";
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
  // The activity types an activity field shows on; empty is every type.
  activityKinds: string[];
};

export function FieldBuilder({
  object,
  ownedColumns = [],
  pending,
  error,
  onSubmit,
  onCancel,
}: Readonly<{
  object: CfObject;
  // The columns the object's fields already own, retired ones included.
  ownedColumns?: readonly string[];
  pending: boolean;
  // The refused create, for what no field of the form can show.
  error: unknown;
  // `refused` hears a refusal of this draft, to put it on its fields.
  onSubmit: (draft: NewFieldDraft, refused: (error: unknown) => void) => void;
  // Leaving the form without adding a field. The builder lives in a dialog, so
  // its secondary verb closes that dialog. A form discarded on close has
  // nothing to reset to.
  onCancel: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
  const labels = {
    label: t("cf.label"),
    type: t("cf.typeLabel"),
    currency: t("cf.currencyCode"),
    activity_kinds: t("cf.activityKinds"),
  };
  const refusal = (refused: unknown) =>
    refusalSentences(refused, t, labels, {
      conflict: {
        field: "label",
        code: "custom_field_label_taken",
        sentence: t("cf.labelTaken"),
      },
    });
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
  const [activityKinds, setActivityKinds] = useState<string[]>([]);
  const structural = looksStructural(label);

  // What the draft needs before it is sent, each on the field that lacks it.
  const checks = () => {
    const found = new Map<string, string>();
    if (label.trim() === "") {
      found.set("label", fieldCheckSentence("required", locale, labels.label));
    }
    const listed = type === "picklist" || type === "multiselect";
    if (listed && !options.some((opt) => opt.trim().length > 0)) {
      found.set("options", t("cf.lastOptionBlocked"));
    }
    if (type === "record" && !kinds.some((kind) => kind.name === options[0])) {
      found.set(
        "options",
        fieldCheckSentence("required", locale, t("cf.recordKind")),
      );
    }
    if (type === "currency" && !/^[A-Za-z]{3}$/.test(currency.trim())) {
      found.set("currency", t("cf.currencyNotCode"));
    }
    return found;
  };

  const confirm = () => {
    if (pending || structural || faults.show(checks())) {
      return;
    }
    onSubmit(
      {
        object,
        label: label.trim(),
        type,
        currency,
        options,
        showInCreate,
        activityKinds: object === "activity" ? activityKinds : [],
      },
      (refused) => faults.show(refusal(refused).byField),
    );
  };
  const rest = refusal(error).rest;

  return (
    <>
      <div className="form-stack" ref={faultsRoot}>
        <div className="cf-builder-head">
          <p className="cf-hint">{t("cf.builder.intro")}</p>
          <Badge>{t("cf.builder.noCode")}</Badge>
        </div>

        <div className="cf-grid">
          <Field label={labels.label} required error={faults.of("label")}>
            {(control) => (
              <TextInput
                {...control}
                value={label}
                onChange={(event) => {
                  setLabel(event.target.value);
                  faults.edited("label");
                }}
              />
            )}
          </Field>
          <Field label={t("cf.apiKey")} hint={t("cf.apiKeyHint")}>
            {(control) => (
              <TextInput
                {...control}
                value={apiKey(object, label, ownedColumns)}
                disabled
                readOnly
              />
            )}
          </Field>
        </div>

        <Field label={labels.type} error={faults.of("type")}>
          {(control) => (
            <Select
              {...control}
              options={offeredTypes.map((one) => ({
                value: one,
                label: typeLabels(t)[one],
              }))}
              value={type}
              onChange={(next) => {
                setType(offeredTypes.find((one) => one === next) ?? type);
                faults.edited("type");
                faults.edited("options");
                faults.edited("currency");
              }}
            />
          )}
        </Field>

        {type === "currency" && (
          <Field
            label={labels.currency}
            hint={t("cf.currencyHint")}
            required
            error={faults.of("currency")}
          >
            {(control) => (
              <TextInput
                {...control}
                value={currency}
                maxLength={3}
                onChange={(event) => {
                  setCurrency(event.target.value.toUpperCase());
                  faults.edited("currency");
                }}
              />
            )}
          </Field>
        )}

        {type === "record" && (
          <Field
            label={t("cf.recordKind")}
            hint={t("cf.recordKindHint")}
            required
            error={faults.of("options")}
          >
            {(control) => (
              <Select
                {...control}
                options={kinds.map((kind) => ({
                  value: kind.name,
                  label: kind.display_name,
                }))}
                value={options[0] ?? ""}
                placeholder={t("cf.recordKindPlaceholder")}
                onChange={(kind) => {
                  setOptions([kind]);
                  faults.edited("options");
                }}
              />
            )}
          </Field>
        )}

        {(type === "picklist" || type === "multiselect") && (
          <OptionRows
            options={options}
            onChange={(next) => {
              setOptions(next);
              faults.edited("options");
            }}
            error={faults.of("options")}
          />
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

        {object === "activity" && (
          <Field
            label={labels.activity_kinds}
            hint={t("cf.activityKindsHint")}
            error={faults.of("activity_kinds")}
          >
            {(control) => (
              <ActivityKindsSelect
                {...control}
                values={activityKinds}
                onChange={(next) => {
                  setActivityKinds(next);
                  faults.edited("activity_kinds");
                }}
              />
            )}
          </Field>
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
        </Callout>
        {rest !== null && <ErrorLine>{rest}</ErrorLine>}
      </div>

      {/* Cancel first, then the verb that writes: the order every dialog in
          this tree uses. Closing the dialog discards the draft, so no reset
          control is needed. */}
      <div className="actions">
        <Button variant="ghost" onClick={onCancel}>
          {t("deals.cancel")}
        </Button>
        <Button
          variant="primary"
          disabled={structural}
          pending={pending}
          onClick={confirm}
        >
          {t("cf.confirm")}
        </Button>
      </div>
    </>
  );
}
