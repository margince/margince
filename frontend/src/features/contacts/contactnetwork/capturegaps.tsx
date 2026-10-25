// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";

import { api } from "../../../api/client";
import { Button } from "../../../design-system/button";
import { Callout } from "../../../design-system/callout";
import { ErrorLine } from "../../../design-system/errorline";
import { formatNumber } from "../../../format/format";
import { useLocale, usePlural, useT } from "../../../i18n";
import { unwrap } from "../../../lib/problem";

/**
 * useContactCaptureGaps reads the colleagues who are on a contact's threads
 * without a connected mailbox. Information only: they are never routes in.
 */
function useContactCaptureGaps(id: string) {
  return useQuery({
    queryKey: ["contact-network", id],
    queryFn: async () =>
      unwrap(
        await api.GET("/contacts/{id}/network", { params: { path: { id } } }),
      ),
  });
}

/**
 * CaptureGapsNotice says that later mail may be missing because a colleague on
 * the threads has not connected a mailbox. It draws nothing when there is none,
 * and nothing while the read is in flight, because the routes above it do not
 * depend on it. A failed read says so: drawing nothing then would claim every
 * colleague's mail is captured.
 */
export function CaptureGapsNotice({
  contactId,
}: Readonly<{ contactId: string }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const network = useContactCaptureGaps(contactId);
  if (network.isError) {
    return (
      <ErrorLine
        actions={
          <Button
            variant="link"
            pending={network.isFetching}
            onClick={() => network.refetch()}
          >
            {t("common.retry")}
          </Button>
        }
      >
        {t("contact.network.captureGapsFailed")}
      </ErrorLine>
    );
  }
  const gaps = network.data?.capture_gaps ?? [];
  if (gaps.length === 0) {
    return null;
  }
  // A server older than the total sends none; the list is then all it knows.
  const total = Math.max(network.data?.capture_gaps_total ?? 0, gaps.length);
  const listed = gaps
    .map((g) =>
      g.invited
        ? t("contact.network.captureGapInvited", { name: g.display_name })
        : g.display_name,
    )
    .join(", ");
  const unlisted = total - gaps.length;
  const names =
    unlisted > 0
      ? plural("contact.network.captureGapMore", unlisted, {
          names: listed,
          count: formatNumber(unlisted, locale),
        })
      : listed;
  return (
    <Callout
      tone="info"
      kind="standing"
      title={plural("contact.network.captureGap", total, { names })}
    />
  );
}
