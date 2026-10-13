// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { formatNumber } from "../format/format";
import { useLocale, usePlural } from "../i18n";

// What an archive's restore could not bring back, said above the history so a
// "done" does not read as a whole record when it is not.
export function LeftBehindNotice({ count }: Readonly<{ count: number }>) {
  const plural = usePlural();
  const { locale } = useLocale();
  if (count === 0) {
    return null;
  }
  return (
    <p role="status">
      {plural("history.undo.leftBehind", count, {
        count: formatNumber(count, locale),
      })}
    </p>
  );
}

// The count belongs to the record it was produced for: a panel that stays
// mounted while the reader moves to another record shows none of it.
export function useLeftBehind(kind: string, id: string) {
  const record = `${kind}:${id}`;
  const [held, setHeld] = useState({ record, count: 0 });
  return {
    count: held.record === record ? held.count : 0,
    report: (count: number) => setHeld({ record, count }),
  };
}
