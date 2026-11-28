// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { api } from "../api/client";
import { useNow } from "../format/now";
import { useT } from "../i18n";
import { unwrap } from "../lib/problem";
import { useMe } from "../lib/session";
import { Button } from "./atoms";
import { OffsiteLink } from "./offsitelink";

export function ChatSourceLink({
  activityId,
  provider,
}: Readonly<{ activityId: string; provider: string }>) {
  const t = useT();
  const account = useMe().data?.user.id;
  const check = useMutation({
    mutationFn: async ({
      activity,
    }: Readonly<{ activity: string; account: string }>) =>
      unwrap(
        await api.GET("/activities/{id}/chat-source", {
          params: { path: { id: activity } },
        }),
      ),
  });
  const now = useNow();
  const sameReader =
    Boolean(account) &&
    check.variables?.account === account &&
    check.variables?.activity === activityId;
  const proof =
    sameReader && check.data?.user_id === account ? check.data : undefined;
  if (
    proof?.available &&
    proof.url &&
    proof.valid_until &&
    Date.parse(proof.valid_until) > now
  ) {
    return (
      <OffsiteLink href={proof.url}>
        {t("timeline.chatSummary.open", { provider })}
      </OffsiteLink>
    );
  }
  return (
    <span>
      {((sameReader && check.isError) || proof?.available === false) && (
        <>
          <span>{t("timeline.chatSummary.sourceUnavailable")}</span>{" "}
          <span>{t("timeline.chatSummary.sourceHelp")}</span>{" "}
        </>
      )}
      <Button
        variant="link"
        disabled={!account || check.isPending}
        aria-busy={check.isPending}
        onClick={() => {
          if (account) check.mutate({ activity: activityId, account });
        }}
      >
        {t(
          check.isPending
            ? "timeline.chatSummary.sourceChecking"
            : "timeline.chatSummary.sourceCheck",
        )}
      </Button>
    </span>
  );
}
