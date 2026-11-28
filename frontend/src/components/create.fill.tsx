// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useRef, useState } from "react";
import { Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { useT } from "../i18n";
import type { CreateField } from "./create.fields";

/**
 * The button under a field that fills it with a suggestion. The suggestion
 * only fills the field; the form's own Save keeps or drops it.
 *
 * The answer lands on the form as it is when it arrives, so edits made while
 * the model ran stay. An answer for source text that has since changed is
 * dropped: it would be a translation of words no longer there.
 */
export function FillButton({
  fill,
  values,
  setValue,
}: Readonly<{
  fill: NonNullable<CreateField["fill"]>;
  values: Record<string, string>;
  setValue: (next: string, latest: Record<string, string>) => void;
}>) {
  const t = useT();
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const latest = useRef({ values, setValue });
  useEffect(() => {
    latest.current = { values, setValue };
  });
  const text = fill.source(values);
  const press = async () => {
    setBusy(true);
    setFailed(false);
    try {
      const suggested = await fill.suggest(text);
      const now = latest.current;
      if (suggested && fill.source(now.values) === text) {
        now.setValue(suggested, now.values);
      }
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
        disabled={busy || text === ""}
        reason={text === "" ? t("translation.suggestNeedsName") : undefined}
        onClick={() => void press()}
      >
        {t(busy ? "translation.suggesting" : fill.label)}
      </Button>
      {failed && <ErrorLine>{t("translation.suggestFailed")}</ErrorLine>}
    </>
  );
}
