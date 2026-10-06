// The folder picker's reads: which mailboxes this seat has, and what is in
// them. Separate from the card because they are the one part that talks to a
// provider rather than to the exclusion store.

import { useMutation, useQueries, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";
import { isMailbox } from "./connectorproviders";
import { useConnectors } from "./connectors";

type CaptureConnection = components["schemas"]["CaptureConnection"];

/**
 * The caller's own connected mailboxes.
 *
 * A container rule names ONE provider's own token, so the picker has to know
 * WHOSE folders it is offering. Reading that from the seat's own connections
 * is what makes the kind work for an Outlook or IMAP mailbox instead of only
 * for Gmail, and it is the same seat-scoped list the server enumerates from.
 *
 * Calendars are left out on the same rule the server applies: a calendar
 * carries no mail, so it has no folder a message could be excluded from.
 */
export function useMailboxes(enabled: boolean) {
  const connectors = useConnectors({ enabled });
  return {
    mailboxes: (connectors.data?.data ?? []).filter(
      (conn) => isMailbox(conn.provider) && conn.status === "connected",
    ),
    isPending: connectors.isPending,
    // A read that failed is not a seat with no mailbox. Without this the
    // dialog would answer "no folders" to somebody whose connections simply
    // could not be listed.
    isError: connectors.isError,
  };
}

/**
 * The folders those mailboxes have, one live read each.
 *
 * Asked only while the container kind is chosen, because it costs a provider
 * round trip: the list is live by design — a folder made this morning is one
 * somebody may want excluded this morning — so it is read when the picker
 * opens rather than kept warm.
 *
 * A provider that does not list folders (501) and a mailbox that is not
 * connected (404) both resolve to NO containers. Neither is a fault the reader
 * can act on, and the kind simply has nothing to offer from that mailbox. Any
 * OTHER answer — an unreachable provider above all — is left to fail, because
 * "your mailbox has no folders" and "we could not ask your mailbox" are
 * different facts and a reader acts on them differently.
 */
export function useContainers(
  mailboxes: readonly CaptureConnection[],
  enabled: boolean,
) {
  return useQueries({
    queries: mailboxes.map((mailbox) => ({
      queryKey: ["connector-containers", mailbox.provider],
      enabled,
      // One round trip per opening of the dialog, not per keystroke.
      staleTime: 60_000,
      retry: false,
      queryFn: async () => {
        const { data, error, response } = await api.GET(
          "/connectors/{provider}/containers",
          { params: { path: { provider: mailbox.provider } } },
        );
        if (response.status === 404 || response.status === 501) {
          return { containers: [] };
        }
        if (error || !data) {
          throwProblem(error);
        }
        return data;
      },
    })),
  });
}

/**
 * Every connected mailbox's folders as one list of options.
 *
 * The value stays provider-qualified, so two mailboxes that both call a folder
 * "Archive" remain two different rules; the label carries the account only
 * when there is more than one mailbox to tell apart.
 */
export function useFolderOptions(enabled: boolean) {
  const {
    mailboxes,
    isPending: mailboxesPending,
    isError: mailboxesFailed,
  } = useMailboxes(enabled);
  const containers = useContainers(mailboxes, enabled);
  return {
    options: mailboxes.flatMap((mailbox, index) =>
      (containers[index]?.data?.containers ?? []).map((container) => ({
        value: `${mailbox.provider}:${container.id}`,
        label:
          mailboxes.length > 1
            ? `${mailbox.account_label ?? mailbox.provider} — ${container.name}`
            : container.name,
      })),
    ),
    isPending: mailboxesPending || containers.some((query) => query.isPending),
    // A provider that did not answer is not a mailbox with no folders — and
    // neither is a list of connections that could not be read at all.
    isError: mailboxesFailed || containers.some((query) => query.isError),
    // ANY mailbox stopping short makes the whole list partial: the options are
    // pooled across accounts, so a reader cannot tell which of them the folder
    // they are hunting for would have come from.
    truncated: containers.some((query) => query.data?.truncated === true),
  };
}

type PurgeOutcome = components["schemas"]["CapturePurgeOutcome"];

/**
 * Destroying the mail a rule already matched.
 *
 * Two calls behind one hook, because the act is two questions: a PREVIEW says
 * what would go, and the purge itself does it. The preview is what makes this
 * safe to offer at all — the destruction is irreversible, and somebody is
 * entitled to see the size of it before they agree.
 *
 * Both answer with the same receipt, so the confirmation and the outcome are
 * read the same way and cannot drift into two vocabularies.
 */
export function usePurgeExclusion() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      id: string;
      preview: boolean;
    }): Promise<PurgeOutcome> => {
      const { data, error } = await api.POST("/capture/exclusions/{id}/purge", {
        params: {
          path: { id: input.id },
          query: { preview: input.preview },
        },
      });
      if (error || !data) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (_outcome, input) => {
      // Only a real purge moved anything. A preview that invalidated the
      // timeline would redraw the screen to say nothing changed, which is
      // true and still reads as something having happened.
      if (!input.preview) {
        queryClient.invalidateQueries({ queryKey: ["capture-exclusions"] });
        queryClient.invalidateQueries({ queryKey: ["activities"] });
      }
    },
  });
}
