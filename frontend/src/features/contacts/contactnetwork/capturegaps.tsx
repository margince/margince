// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";

import { api } from "../../../api/client";
import { Callout } from "../../../design-system/callout";
import { usePlural, useT } from "../../../i18n";
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
 * the threads has not connected a mailbox. It draws nothing when there is none.
 * It also draws nothing while the read is in flight or refused, because the
 * routes above it do not depend on it.
 */
export function CaptureGapsNotice({
  contactId,
}: Readonly<{ contactId: string }>) {
  const t = useT();
  const plural = usePlural();
  const gaps = useContactCaptureGaps(contactId).data?.capture_gaps ?? [];
  if (gaps.length === 0) {
    return null;
  }
  const names = gaps
    .map((g) =>
      g.invited
        ? t("contact.network.captureGapInvited", { name: g.display_name })
        : g.display_name,
    )
    .join(", ");
  return (
    <Callout
      tone="info"
      kind="standing"
      title={plural("contact.network.captureGap", gaps.length, { names })}
    />
  );
}
