// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

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
