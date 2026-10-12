// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { unwrap } from "./common";

// Whether the model lanes are answering.
//
// The binding card above says which vendor serves a tier and the keys card says
// whether this installation can call it. Neither says whether it ANSWERED, and
// under the capture posture that gap is expensive: a thread stays held whether
// the classifier judged it confidential or never replied at all, so an outage
// and correct cautious behaviour look identical until somebody asks why a
// thread never opened.

// The one health read, folded into the Model tiers table.
export function useAiHealth(canSee: boolean) {
  return useQuery({
    queryKey: ["ai-health"],
    enabled: canSee,
    queryFn: async () => {
      return unwrap(await api.GET("/ai/health"));
    },
    // The question is whether it is answering NOW, so a reader who leaves this
    // page open watches it rather than reading a snapshot from when they
    // arrived. One minute against a one-hour window: often enough to notice a
    // lane die, rare enough to cost nothing.
    //
    // Off with the grant, not merely disabled with it: `enabled` stops the poll
    // today, and an interval left standing is what would resume it the moment
    // the flag flipped mid-session.
    refetchInterval: canSee ? 60_000 : false,
  });
}
