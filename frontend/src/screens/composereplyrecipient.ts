// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { unwrap } from "./common";

/**
 * Who a reply to this anchor goes to, resolved without drafting.
 *
 * Sending REQUIRES `to`, so an empty field is not a convenience gap: it is a
 * reply the reader must address by hand against a record that already holds
 * the answer. The server ranks the message's participants — its sender before
 * anyone copied on it — and this asks that same resolution the draft uses, so
 * what the composer shows on open and what a draft fills in cannot disagree.
 *
 * Undefined while unsettled and for a contact with no address on record. The
 * caller only ever fills an EMPTY field from it, so a reader who typed their
 * own recipient keeps it.
 */
export function useReplyRecipient(anchor: string | undefined): {
  address: string | undefined;
  mailboxes: string[] | undefined;
} {
  const query = useQuery({
    queryKey: ["compose-reply-recipient", anchor],
    queryFn: async () => {
      return unwrap(
        await api.GET("/activities/{id}/reply-recipient", {
          params: { path: { id: anchor ?? "" } },
        }),
      );
    },
    enabled: anchor !== undefined,
  });
  // A failed refetch keeps the last answer in `data`; a read that failed is
  // not one to prefill a To field from.
  const answer = query.isError ? undefined : query.data;
  return {
    address: answer?.address === "" ? undefined : answer?.address,
    // Undefined until this ANCHOR's own answer arrives — the query is keyed on
    // it, so a previous thread's settled answer is never served here. That
    // matters: undefined means "not answered yet", which is a different fact
    // from an empty list, and only the empty list says nobody's mailbox.
    mailboxes: answer?.mailbox_user_ids,
  };
}
