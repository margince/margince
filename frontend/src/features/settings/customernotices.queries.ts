// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import { unwrap } from "../../lib/problem";

export const NOTICES_KEY = ["existing-customer-notices"] as const;

export function useMarketingPurposes() {
  return useQuery({
    queryKey: ["consent-purposes"],
    queryFn: async () => unwrap(await api.GET("/consent-purposes")),
    select: (page) => page.data.filter((p) => p.class === "marketing"),
  });
}
