// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { AppLink } from "../../design-system/applink";
import { Callout } from "../../design-system/callout";
import { linkHref } from "../../design-system/richtext-html";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

export function bookingConsentWording(
  t: (key: MessageKey) => string,
  address?: string,
): string {
  const wording = t("book.consentWording");
  return address
    ? `${wording}\n${t("book.privacyInformation")}: ${address}`
    : wording;
}

export function BookingPrivacyInformation({
  address,
}: Readonly<{ address?: string }>) {
  const t = useT();
  if (!address) return null;
  const href = linkHref(address);
  return (
    <p>
      {t("book.privacyInformation")}:{" "}
      {href ? (
        <AppLink href={href} target="_blank" rel="noopener noreferrer">
          {address}
        </AppLink>
      ) : (
        address
      )}
    </p>
  );
}

export function BookingPrivacyWarning({
  address,
}: Readonly<{ address?: string }>) {
  const t = useT();
  return address ? null : (
    <Callout kind="standing" title={t("scheduling.privacyMissing")} />
  );
}
