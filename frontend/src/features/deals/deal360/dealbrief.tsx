import { useState } from "react";
import type { CreateField } from "../../../components/create";
import { type RailEdit, useRailText } from "../../../components/railfield";
import { Button } from "../../../design-system/atoms";
import {
  FieldEditDialog,
  useFieldEditor,
} from "../../../design-system/fieldeditdialog";
import { InlineTextControl } from "../../../design-system/inlinetext";
import {
  OverviewCard,
  OverviewText,
} from "../../../design-system/overviewcard";
import { useT } from "../../../i18n";
import "./deal360.css";

/** How much of the brief shows before the reader asks for the rest. */
const COLLAPSED_LINES = 6;

/**
 * What this deal is, in the words a colleague typed. Nothing generated
 * overwrites it, which is why it stands apart from How it stands above it.
 * An unbriefed deal keeps the card and says the brief is empty: a card that
 * vanished would read as a product with no such field.
 */
export function DealBrief({
  brief,
  edit,
}: Readonly<{
  brief?: string | null;
  // The deal's field edit; absent while the deal takes no writes from this
  // reader.
  edit?: RailEdit;
}>) {
  const t = useT();
  const [expanded, setExpanded] = useState(false);
  const text = brief?.trim();
  // A line clamp, so the count is of lines. A long single paragraph wraps
  // and is clamped, and its length alone would offer "Read more" on text
  // already fully shown.
  const clampable =
    Boolean(text) && (text ?? "").split("\n").length > COLLAPSED_LINES;
  const clamped = clampable && !expanded;
  return (
    <OverviewCard
      title={t("deal.brief.title")}
      action={
        edit && (
          <BriefAction
            edit={edit}
            verb={text ? t("deal.briefEdit") : t("deal.briefAdd")}
          />
        )
      }
      empty={text ? undefined : t("deal.briefEmpty")}
    >
      {text ? (
        <>
          <OverviewText>
            <span
              className={
                clamped ? "d360-brief d360-brief-clamped" : "d360-brief"
              }
              style={clamped ? { WebkitLineClamp: COLLAPSED_LINES } : undefined}
            >
              {text}
            </span>
          </OverviewText>
          {clampable && (
            <Button
              variant="link"
              onClick={() => setExpanded((open) => !open)}
              aria-expanded={expanded}
            >
              {expanded ? t("deal.briefLess") : t("deal.briefMore")}
            </Button>
          )}
        </>
      ) : undefined}
    </OverviewCard>
  );
}

function BriefAction({
  edit,
  verb,
}: Readonly<{ edit: RailEdit; verb: string }>) {
  const field = edit.fields.find((one) => one.key === "description");
  return edit.canEdit && field ? (
    <BriefEditor edit={edit} field={field} verb={verb} />
  ) : null;
}

// The card's link opens the brief's own editor, saved like the details row.
function BriefEditor({
  edit,
  field,
  verb,
}: Readonly<{ edit: RailEdit; field: CreateField; verb: string }>) {
  const t = useT();
  const { stored, canEdit, onEditingChange, onDirtyChange, onSave } =
    useRailText(edit, field);
  const { open, editing } = useFieldEditor(canEdit, onEditingChange);
  return (
    <>
      <Button variant="link" onClick={() => editing(true)}>
        {verb}
      </Button>
      <FieldEditDialog
        open={open}
        label={t("deal.brief")}
        value={stored}
        hint={field.hint}
        onClose={() => editing(false)}
        onSave={onSave}
        onDirtyChange={onDirtyChange}
      >
        {(control) => (
          <InlineTextControl
            field={control}
            multiline
            maxLength={field.maxLength}
          />
        )}
      </FieldEditDialog>
    </>
  );
}
