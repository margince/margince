import { Button, TextInput } from "../../design-system/atoms";
import { EvidenceMark } from "../../design-system/evidencemark";
import { formatMoney } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import {
  AMOUNT_FIELD,
  CONFIDENCE,
  type ExtractedField,
  FIELD_LABELS,
  majorUnits,
  OMITTED_REASONS,
  type OmittedField,
} from "./documentextraction";

// One grounded field: its value carrying the mark that opens onto the quote it
// was read from, and a way to type over it.
//
// The evidence sits on the value, not beside it: that is the design
// language's one provenance affordance. Three chips under a number read as
// clutter, and the number gets lost among them.
export function GroundedField({
  field,
  currency,
  draft,
  onEdit,
  canEdit,
}: Readonly<{
  field: ExtractedField;
  currency: string;
  draft: string | undefined;
  onEdit: (value: string) => void;
  canEdit: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const label = FIELD_LABELS[field.field];
  const money = field.field === AMOUNT_FIELD && currency !== "";
  const shown = money
    ? formatMoney(Number(field.value), currency, locale)
    : field.value;
  return (
    <li>
      <span>{label ? t(label) : field.field}</span>
      {draft === undefined ? (
        <EvidenceMark
          value={shown}
          source={{
            // The agent's job name without the `agent:` prefix, which the tag
            // reads through the catalogue. This value never went through
            // provenanceOf, so it carries no prefix to strip.
            provenance: { kind: "agent", job: "document-extractor" },
            confidence: CONFIDENCE[field.confidence],
            snippet: field.source_quote,
            at: null,
          }}
        />
      ) : (
        <TextInput
          aria-label={t("extraction.editValue", {
            field: label ? t(label) : field.field,
          })}
          value={draft}
          onChange={(event) => onEdit(event.target.value)}
        />
      )}
      <span className="t-caption">{field.page_or_section}</span>
      {canEdit && draft === undefined && (
        <Button
          variant="ghost"
          onClick={() =>
            onEdit(money ? majorUnits(field.value, currency) : field.value)
          }
        >
          {t("extraction.edit")}
        </Button>
      )}
    </li>
  );
}

// What the document does not say, said out loud.
//
// An omission is an answer, not an absence. A rep acts on "this order form
// states no close date", so the panel renders it instead of leaving the reader
// to notice the gap.
export function OmittedList({
  omitted,
}: Readonly<{ omitted: readonly OmittedField[] }>) {
  const t = useT();
  if (omitted.length === 0) {
    return null;
  }
  return (
    <ul>
      {omitted.map((field) => {
        const label = FIELD_LABELS[field.field];
        return (
          <li key={field.field}>
            {label ? t(label) : field.field} —{" "}
            {t(OMITTED_REASONS[field.reason])}
          </li>
        );
      })}
    </ul>
  );
}
