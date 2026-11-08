// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";

/**
 * Whether the save bar has anything in it. `Panel` draws its footer band for
 * any node, so the caller asks this first and never draws an empty band.
 */
export function hasFoot(
  reading: Readonly<{ editable: boolean; staged: number; unsaved: number }>,
): boolean {
  return reading.unsaved > 0 || (reading.editable && reading.staged > 0);
}

/**
 * The save bar, and the refusal that can stand where it was.
 *
 * A refusal stays after `editable` goes false: the week closing is what
 * refused the save, so the reader keeps the reason. The button goes, because
 * the rows lose their boxes and a Save would send ticks nobody can see.
 */
export function PlanFoot({
  editable,
  staged,
  unsaved,
  saving,
  onSave,
}: Readonly<{
  editable: boolean;
  staged: number;
  unsaved: number;
  saving: boolean;
  onSave: () => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <>
      {/* `danger`: the write was refused, not merely at risk. */}
      {unsaved > 0 && (
        <Callout tone="danger" kind="outcome" title={t("plan.refusedTitle")}>
          {plural("plan.saveRefused", unsaved, {
            count: formatNumber(unsaved, locale),
          })}
        </Callout>
      )}
      {/* `pending`, not `disabled`: a write in flight is not a precondition the
          reader can meet, and `disabled` is native — it drops focus from the
          control they just pressed. */}
      {editable && staged > 0 && (
        <Button onClick={onSave} pending={saving}>
          {plural("plan.save", staged, { count: formatNumber(staged, locale) })}
        </Button>
      )}
    </>
  );
}
