// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryStates } from "../../design-system/querystates";
import { useT } from "../../i18n";
import { type MessengerScope, useMessengerTeam } from "./messenger.queries";

export function MessengerTeamComparison({
  members,
  access,
  scope,
}: Readonly<{
  members: readonly { id: string; name: string }[];
  access?: string;
  scope: MessengerScope;
}>) {
  const t = useT();
  const team = useMessengerTeam(scope);
  const assigned = team.data?.members ?? [];
  const verified = assigned.filter(
    (member) => member.state === "matched" || member.state === "self",
  );
  const expected = new Set(verified.map((member) => member.member_id));
  const recipients = new Set(members.map((member) => member.id));
  const outside = members.filter((member) => !expected.has(member.id));
  const missing = verified.filter(
    (member) => !recipients.has(member.member_id),
  );
  const unresolved = assigned.filter(
    (member) => member.state !== "matched" && member.state !== "self",
  );
  return (
    <QueryStates query={team} pendingLabel={t("messenger.loading")}>
      {team.data && (
        <section>
          <p>{t("messenger.teamComparison")}</p>
          {assigned.length === 0 && <p>{t("messenger.noTeam")}</p>}
          {outside.length > 0 && (
            <>
              <p>{t("messenger.teamOutside")}</p>
              <ul>
                {outside.map((member) => (
                  <li key={member.id}>{member.name}</li>
                ))}
              </ul>
            </>
          )}
          {missing.length > 0 && (
            <>
              <p>
                {t(
                  access === "workspace"
                    ? "messenger.teamNotJoined"
                    : "messenger.teamMissing",
                )}
              </p>
              <ul>
                {missing.map((member) => (
                  <li key={`${member.kind}:${member.crm_id}`}>{member.name}</li>
                ))}
              </ul>
            </>
          )}
          {unresolved.length > 0 && (
            <>
              <p>{t("messenger.teamUnverified")}</p>
              <ul>
                {unresolved.map((member) => (
                  <li key={`${member.kind}:${member.crm_id}`}>{member.name}</li>
                ))}
              </ul>
            </>
          )}
          {verified.length > 0 &&
            outside.length === 0 &&
            missing.length === 0 &&
            unresolved.length === 0 && <p>{t("messenger.teamMatches")}</p>}
        </section>
      )}
    </QueryStates>
  );
}
