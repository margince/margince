import { ShieldAlert } from "lucide-react";
import { Field } from "../../design-system/atoms";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import {
  asksWhy,
  type CommunicationContext,
  contextOptions,
} from "../../lib/compose-context";
import type { Activity } from "./compose";
import type { MissingField } from "./composesend";

// Why the reader is writing, asked where the record does not answer it. A
// reply derives its category from the thread it answers, which the checks say.
export function ComposeWhy({
  anchorActivity,
  needsReason,
  flagged,
  context,
  onContextChange,
}: Readonly<{
  anchorActivity: Activity | undefined;
  /** The check could not settle why a reply is written, or a reason is set. */
  needsReason: boolean;
  flagged: ReadonlySet<MissingField>;
  context: CommunicationContext | "";
  onContextChange: (next: CommunicationContext | "") => void;
}>) {
  const t = useT();
  if (!asksWhy(anchorActivity, needsReason)) {
    return null;
  }
  const missing = context === "";
  // One strip in the warning band: what is missing and why it matters, with
  // the pick that fixes it at the end. Once picked it stays, without the warning.
  return (
    <div
      className={
        missing ? "compose-strip compose-strip-warning" : "compose-strip"
      }
    >
      {missing && <ShieldAlert aria-hidden="true" />}
      <p className="compose-strip-text">
        {missing ? (
          <>
            <strong>{t("compose.whyMissing")}</strong>{" "}
            {t("compose.whyMissingText")}
          </>
        ) : (
          t("compose.why")
        )}
      </p>
      <Field
        className="compose-strip-field"
        label={t("compose.why")}
        labelHidden
        error={flagged.has("context") ? t("compose.missingWhy") : undefined}
      >
        {(control) => (
          <Select
            {...control}
            options={contextOptions(t).filter((o) => o.value !== "")}
            value={context}
            placeholder={t("compose.whyPick")}
            onChange={(value) =>
              onContextChange(value as CommunicationContext | "")
            }
          />
        )}
      </Field>
    </div>
  );
}
