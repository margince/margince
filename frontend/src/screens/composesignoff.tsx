// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useT } from "../i18n";
import { throwProblem, useMe } from "./common";

type SignOff = components["schemas"]["EmailSignOff"];

// How long the words must rest before the closing's language is asked again.
const SETTLE_MS = 400;

/**
 * The cache family every sign-off preview lives under. Saving a signature
 * invalidates the whole family; each entry is keyed by the user it was read
 * for, because it holds their signature and name.
 */
export const SIGN_OFF_QUERY = "email-sign-off";

/**
 * The block a send appends under the body, read-only, as the server will
 * append it. The server is asked rather than the signature read here: a sender
 * with no signature gets a closing in the message's language, and only the
 * send path knows which.
 */
export function SignOffPreview({
  body,
  subject,
}: Readonly<{ body: string; subject: string }>) {
  const t = useT();
  const { signOff, stale } = useSignOff(body, subject);
  if (!signOff?.text || signOff.kind === "none") {
    return null;
  }
  // While the words have moved on and the answer has not, the block is drawn
  // as updating: the closing's language may be about to change.
  return (
    <section
      className={stale ? "compose-signoff is-stale" : "compose-signoff"}
      aria-label={t("compose.signOff")}
      aria-busy={stale}
    >
      <span className="t-caption">{t("compose.signOff")}</span>
      <p className="compose-signoff-text">{signOff.text}</p>
      {signOff.kind === "closing" && (
        <p className="t-caption">
          {t("compose.signOffClosing")}{" "}
          <a href="#/settings/account">{t("compose.signOffSet")}</a>
        </p>
      )}
    </section>
  );
}

function useSignOff(
  body: string,
  subject: string,
): { signOff: SignOff | undefined; stale: boolean } {
  const userId = useMe().data?.user.id;
  const [settled, setSettled] = useState({ body, subject });
  useEffect(() => {
    const timer = setTimeout(() => setSettled({ body, subject }), SETTLE_MS);
    return () => clearTimeout(timer);
  }, [body, subject]);
  const query = useQuery({
    queryKey: [SIGN_OFF_QUERY, userId, settled.body, settled.subject],
    queryFn: async () => {
      const { data, error } = await api.POST("/emails:sign-off", {
        body: settled,
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    enabled: userId !== undefined,
    // The previous answer stays while the next is asked, so the block does not
    // blink out on every pause in typing — but only the SAME user's: another
    // member's signature is never a placeholder for this one's.
    placeholderData: (previous, previousQuery) =>
      previousQuery?.queryKey[1] === userId ? previous : undefined,
  });
  const pending = settled.body !== body || settled.subject !== subject;
  return { signOff: query.data, stale: pending || query.isPlaceholderData };
}
