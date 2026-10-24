// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { PenLine } from "lucide-react";
import { useEffect, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { SignatureHtml } from "../../components/signaturehtml";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { routeHash } from "../../lib/router";
import { useMe } from "../../lib/session";
import { settingsHref } from "../../lib/settingsrouting";

type SignOff = components["schemas"]["EmailSignOff"];
type SignatureDraft = components["schemas"]["EmailSignatureDraft"];

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
  const { signOff, stale, failed } = useSignOff(body, subject);
  if (failed) {
    return (
      <p className="t-caption compose-signoff">{t("compose.signOffFailed")}</p>
    );
  }
  if (!signOff?.text || signOff.kind === "none") {
    return null;
  }
  // While the words have moved on and the answer has not, the block is drawn
  // as updating: the closing's language may be about to change.
  const className = stale ? "compose-signoff is-stale" : "compose-signoff";
  // A closing is one line of the product's own words, said in a sentence with
  // the way to replace it. A signature is the sender's own block, shown whole.
  if (signOff.kind === "closing") {
    return (
      <section
        className={className}
        aria-label={t("compose.signOff")}
        aria-busy={stale}
      >
        <PenLine aria-hidden="true" />
        <p className="t-caption">
          {t("compose.signsOffAs", {
            closing: signOff.text
              .trim()
              .split(/\s*\n\s*/)
              .join(" "),
          })}{" "}
          {t("compose.signOffClosing")}
        </p>
        <a href={routeHash(settingsHref("account"))}>
          {t("compose.signOffSet")}
        </a>
      </section>
    );
  }
  return (
    <section
      className={`${className} is-signature`}
      aria-label={t("compose.signOff")}
      aria-busy={stale}
    >
      <span className="t-caption">{t("compose.signOff")}</span>
      {signOff.html ? (
        <SignatureHtml html={signOff.html} title={t("compose.signOff")} />
      ) : (
        <p className="compose-signoff-text">{signOff.text}</p>
      )}
    </section>
  );
}

export function useSignOff(
  body: string,
  subject: string,
  draft?: SignatureDraft,
): { signOff: SignOff | undefined; stale: boolean; failed: boolean } {
  const userId = useMe().data?.user.id;
  // A settings form's unsaved values, compared by content: the form builds a
  // new object on every render.
  const draftKey = draft === undefined ? "" : JSON.stringify(draft);
  const [settled, setSettled] = useState({ body, subject, draftKey });
  useEffect(() => {
    const timer = setTimeout(
      () => setSettled({ body, subject, draftKey }),
      SETTLE_MS,
    );
    return () => clearTimeout(timer);
  }, [body, subject, draftKey]);
  const query = useQuery({
    queryKey: [
      SIGN_OFF_QUERY,
      userId,
      settled.body,
      settled.subject,
      settled.draftKey,
    ],
    queryFn: async () => {
      return unwrap(
        await api.POST("/emails:sign-off", {
          body: {
            body: settled.body,
            subject: settled.subject,
            ...(settled.draftKey === ""
              ? {}
              : { draft: parseDraft(settled.draftKey) }),
          },
        }),
      );
    },
    enabled: userId !== undefined,
    // The previous answer stays while the next is asked, so the block does not
    // blink out on every pause in typing — but only the SAME user's: another
    // member's signature is never a placeholder for this one's.
    placeholderData: (previous, previousQuery) =>
      previousQuery?.queryKey[1] === userId ? previous : undefined,
  });
  const pending =
    settled.body !== body ||
    settled.subject !== subject ||
    settled.draftKey !== draftKey;
  return {
    signOff: query.data,
    stale: pending || query.isPlaceholderData,
    failed: query.isError,
  };
}

function parseDraft(key: string): SignatureDraft {
  const parsed: SignatureDraft = JSON.parse(key);
  return parsed;
}
