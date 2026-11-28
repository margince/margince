// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { QueryStates } from "../../design-system/querystates";
import { formatNumber } from "../../format/format";
import { type Translator, useLocale, useT } from "../../i18n";
import { useCanMutate } from "../../lib/capability";
import { problemMessageOf } from "../../lib/problem";
import {
  type MessengerScope,
  useMessengerOpenPeople,
  useMessengerSetup,
  useMessengerTeam,
} from "./messenger.queries";

export function MessengerTeamPicker({
  scope,
  onChoose,
}: Readonly<{ scope: MessengerScope; onChoose: (id: string) => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const canMutate = useCanMutate();
  const team = useMessengerTeam(scope);
  const setup = useMessengerSetup(scope.provider);
  const open = useMessengerOpenPeople();
  const members = team.data?.members ?? [];
  const others = members.filter((person) => person.state === "matched");
  const unresolved = members.some(
    (person) => person.state !== "matched" && person.state !== "self",
  );
  const duplicate =
    new Set(others.map((person) => person.member_id)).size !== others.length;
  const limit = setup.data?.max_group_recipients ?? 0;
  const tooMany = others.length > limit;
  return (
    <div className="messenger-picker">
      <QueryStates query={team} pendingLabel={t("messenger.loading")}>
        {team.data && (
          <>
            {members.length === 0 && <p>{t("messenger.noTeam")}</p>}
            <ul>
              {members.map((person) => (
                <li key={`${person.kind}:${person.crm_id}`}>
                  {person.name} — {t(recipientState(person.state))}
                </li>
              ))}
            </ul>
            <QueryStates query={setup} pendingLabel={t("messenger.loading")}>
              <Button
                variant="primary"
                pending={open.isPending}
                disabled={
                  !canMutate ||
                  unresolved ||
                  duplicate ||
                  others.length === 0 ||
                  tooMany ||
                  !setup.data
                }
                reason={teamRefusal(t, {
                  canMutate,
                  unresolved: unresolved || duplicate,
                  tooMany,
                  limit: formatNumber(limit, locale),
                  empty: others.length === 0,
                  loading: !setup.data,
                })}
                onClick={() =>
                  open.mutate(
                    {
                      scope,
                      members: others.map((person) => person.member_id),
                    },
                    {
                      onSuccess: (context) => onChoose(context.conversation.id),
                    },
                  )
                }
              >
                {t("messenger.messageTeam")}
              </Button>
            </QueryStates>
          </>
        )}
      </QueryStates>
      {open.error && (
        <Callout tone="warning" title={t("messenger.loadProblem")}>
          {problemMessageOf(open.error, t)}
        </Callout>
      )}
    </div>
  );
}

function recipientState(state: "matched" | "self" | "unresolved" | "inactive") {
  switch (state) {
    case "matched":
      return "messenger.matched";
    case "self":
      return "messenger.you";
    case "inactive":
      return "messenger.inactive";
    default:
      return "messenger.unresolved";
  }
}

function teamRefusal(
  t: Translator,
  state: Readonly<{
    canMutate: boolean;
    unresolved: boolean;
    tooMany: boolean;
    limit: string;
    empty: boolean;
    loading: boolean;
  }>,
) {
  if (!state.canMutate) return t("settings.seatRefused");
  if (state.unresolved) return t("messenger.teamUnresolved");
  if (state.tooMany) return t("messenger.groupLimit", { limit: state.limit });
  if (state.empty) return t("messenger.noTeam");
  if (state.loading) return t("messenger.loading");
  return undefined;
}
