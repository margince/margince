// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { useT } from "../i18n";
import type { CreateField } from "./create.fields";

/**
 * The button under a field that fills it with a suggestion. The suggestion
 * only fills the field; the form's own Save keeps or drops it.
 */
export function FillButton({
  fill,
  values,
  setValue,
}: Readonly<{
  fill: NonNullable<CreateField["fill"]>;
  values: Record<string, string>;
  setValue: (next: string) => void;
}>) {
  const t = useT();
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const press = async () => {
    setBusy(true);
    setFailed(false);
    try {
      const suggested = await fill.from(values);
      if (suggested) setValue(suggested);
    } catch {
      setFailed(true);
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      <Button
        type="button"
        variant="aiQuiet"
        disabled={busy || !fill.ready(values)}
        onClick={() => void press()}
      >
        {t(busy ? "translation.suggesting" : fill.label)}
      </Button>
      {failed && <ErrorLine>{t("translation.suggestFailed")}</ErrorLine>}
    </>
  );
}
