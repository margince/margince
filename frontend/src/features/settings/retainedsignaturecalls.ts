// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { saveIndependentEdit } from "../../lib/independentedit";
import { unwrap } from "../../lib/problem";

export type ReadySignature = components["schemas"]["RetainedEmailSignature"] & {
  version: number;
};
export type SignatureValues = Pick<
  components["schemas"]["EmailSignature"],
  "body" | "html" | "title" | "phone"
>;

export function signatureValues(signature: ReadySignature): SignatureValues {
  const { body, html, title, phone } = signature.signature;
  return { body, html, title, phone };
}

function readySignature(
  signature: components["schemas"]["RetainedEmailSignature"] | undefined,
  missingVersion: string,
): ReadySignature {
  if (signature === undefined || signature.version === undefined)
    throw new Error(missingVersion);
  return { ...signature, version: signature.version };
}

export async function readRetainedSignature(
  id: string,
  missingVersion: string,
  signal?: AbortSignal,
): Promise<ReadySignature> {
  return readySignature(
    unwrap(
      await api.GET("/privacy/retained-signatures/{id}", {
        signal,
        params: { path: { id } },
      }),
    ),
    missingVersion,
  );
}

export async function saveRetainedReview(
  original: ReadySignature,
  values: SignatureValues,
  missingVersion: string,
  receivedLatest: (signature: ReadySignature) => void,
): Promise<ReadySignature> {
  let current = original;
  return saveIndependentEdit({
    opened: { id: original.id, original },
    patch: values,
    project: signatureValues,
    read: async () => {
      current = await readRetainedSignature(original.id, missingVersion);
      receivedLatest(current);
      return current;
    },
    write: async (changes, version) =>
      readySignature(
        unwrap(
          await api.PUT("/privacy/retained-signatures/{id}", {
            params: { path: { id: original.id } },
            body: {
              ...signatureValues(current),
              ...changes,
              expected_version: version,
            },
          }),
        ),
        missingVersion,
      ),
  });
}
