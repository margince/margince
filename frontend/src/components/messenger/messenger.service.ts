// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import { unwrap } from "../../lib/problem";

export function useMessengerService() {
  return useQuery({
    queryKey: ["messenger-service"],
    queryFn: async () => unwrap(await api.GET("/messenger/service")),
    retry: false,
    staleTime: 15000,
  });
}
