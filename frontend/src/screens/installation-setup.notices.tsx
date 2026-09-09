// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Callout } from "../design-system/callout";
import { useT } from "../i18n";
import { problemMessageOf } from "./common";

// What a cold start says when one of its two writes was refused.
//
// Their own file because each notice's whole job is to name WHICH write failed,
// and that reasoning does not belong in the middle of a step's form: an
// operator meeting "something went wrong" on the first screen of a fresh
// installation has nothing else on the page to work it out from.

/**
 * The model step: the key is sealed first, then the binding is written, so at
 * most one of the two is refused at a time.
 *
 * The heading names which. One sentence over `saveKey.error ?? bind.error` left
 * the reader unable to tell whether their credential was rejected — retype it —
 * or accepted and the binding refused, which is a different remedy entirely.
 */
export function AiBindRefused({
  keyError,
  bindError,
}: Readonly<{ keyError: unknown; bindError: unknown }>) {
  const t = useT();
  const failure = keyError ?? bindError;
  if (!failure) {
    return null;
  }
  return (
    <Callout
      tone="danger"
      kind="outcome"
      title={t(keyError ? "firstRun.ai.keyFailed" : "firstRun.ai.bindFailed")}
    >
      {problemMessageOf(failure, t)}
    </Callout>
  );
}

/** The organisation's OAuth app was not stored. */
export function AppSaveRefused({ error }: Readonly<{ error: unknown }>) {
  const t = useT();
  if (!error) {
    return null;
  }
  return (
    <Callout tone="danger" kind="outcome" title={t("oauthApp.saveFailed")}>
      {problemMessageOf(error, t)}
    </Callout>
  );
}
