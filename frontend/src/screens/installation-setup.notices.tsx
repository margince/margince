// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { WriteRefused } from "./common";

// What a cold start says when the model step's write was refused.
//
// Its own file because the notice's whole job is to name WHICH write failed,
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
  // A wrapper over the shared notice, and the heading is the whole difference:
  // which of the two writes failed is a question only this step can answer.
  return (
    <WriteRefused
      titleKey={keyError ? "firstRun.ai.keyFailed" : "firstRun.ai.bindFailed"}
      error={keyError ?? bindError}
    />
  );
}
