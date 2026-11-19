// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Card } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { formatTimeOfDay } from "../../format/format.dates";
import { useLocale, useT } from "../../i18n";
import { useViewerZone } from "../../lib/viewerzone";

// A clear morning, said once and warmly. Only a complete read may draw it:
// the caller keeps a partial or failed read on its own sentences.
export function CaughtUp({ asOf }: Readonly<{ asOf: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  return (
    <Card as="div" className="brief-caughtup">
      <svg
        className="brief-caughtup-mark"
        aria-hidden="true"
        viewBox="0 0 96 96"
      >
        <circle className="brief-caughtup-halo" cx="48" cy="48" r="48" />
        <circle className="brief-caughtup-ring" cx="48" cy="48" r="40" />
        <circle className="brief-caughtup-disc" cx="48" cy="48" r="32" />
        <path
          className="brief-caughtup-tick"
          pathLength={1}
          d="M58.7 40 44 54.7l-6.7-6.7"
        />
      </svg>
      <Heading size="medium">{t("brief.feed.caughtUp")}</Heading>
      <p className="brief-caughtup-line">
        {t("brief.feed.caughtUpDetail", {
          time: formatTimeOfDay(asOf, locale, zone),
        })}
      </p>
    </Card>
  );
}
