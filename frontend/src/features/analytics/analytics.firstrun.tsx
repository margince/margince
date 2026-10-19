// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import { Button, EmptyState } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { useCan } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import { navigate } from "../../lib/router";

// An installation with no deals yet. Its reports are empty because nothing
// has been recorded. A row of empty charts does not say that, so the page
// names the first step instead.
export function FirstRunNotice() {
  const t = useT();
  const canReadDeals = useCan("deal", "read");
  const probe = useQuery({
    queryKey: ["deals", "first-run"],
    enabled: canReadDeals,
    queryFn: async () =>
      unwrap(await api.GET("/deals", { params: { query: { limit: 1 } } })),
  });
  if (!canReadDeals || !probe.data || probe.data.data.length > 0) {
    return null;
  }
  return (
    <EmptyState
      title={t("analytics.firstRunTitle")}
      action={
        <Button onClick={() => navigate({ screen: "deals" })}>
          {t("analytics.firstRunAction")}
        </Button>
      }
    >
      {t("analytics.firstRunBody")}
    </EmptyState>
  );
}
