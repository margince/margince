import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

// Asking the engine whether a message would be allowed, before anybody presses
// Send.
//
// ONE hook for every surface that stages a send. The composer, the scheduled
// send and the held-draft approval card each ask the same question about the
// same message, and each having its own fetch is how two surfaces come to
// disagree about one message — one saying it will go, the other that it will
// not, with no way to tell which is right.
//
// It asks the SERVER rather than reasoning from the record it has on screen.
// The answer depends on evidence a composer cannot see — a suppression row, a
// thread it is not showing, a jurisdiction's ceiling — so a surface that
// guessed would be a second implementation of the authorization engine.

type Preview = components["schemas"]["SendAuthorizationPreview"];

/**
 * What the engine says about a message as it currently stands.
 *
 * `anchorActivityId` selects the door: a reply is previewed against the thread
 * it answers, which is what lets the engine resolve it from the anchor's own
 * evidence. Without one the message is a fresh send and carries no thread.
 */
export function useSendPermission(args: {
  recipients: readonly string[];
  anchorActivityId?: string;
  /**
   * The records the message is filed under. Required by the account-send door
   * and ignored by the reply door, which resolves from its anchor thread.
   */
  links?: readonly components["schemas"]["ActivityLinkInput"][];
  context?: components["schemas"]["CommunicationContext"];
  /**
   * Which marketing purpose, when the claim is `marketing`. Part of the
   * question: a marketing send previewed without it is asked about a
   * different message than the one that will go.
   */
  marketingPurpose?: string;
  /** Off while the rep is still choosing a recipient — nothing to ask about yet. */
  enabled?: boolean;
}): { preview: Preview | undefined; asking: boolean; unanswered: boolean } {
  const recipients = args.recipients.filter((address) => address.trim() !== "");
  const links = args.links ?? [];
  // A preview with no recipient answers about nobody, and the account-send door
  // REFUSES a request naming no records — it answers 422 before the engine sees
  // it. Firing anyway would spend a request to be refused, and the failure
  // would render as silence, which reads exactly like "allowed" on the cold
  // sends where consent matters most.
  const askable =
    recipients.length > 0 &&
    (args.anchorActivityId !== undefined || links.length > 0);
  const enabled = (args.enabled ?? true) && askable;

  const query = useQuery({
    // The recipients, the records and the claimed context are the whole
    // question, so they are the whole key: changing any of them must re-ask
    // rather than serve the answer to a different message.
    queryKey: [
      "send-permission",
      args.anchorActivityId ?? "",
      recipients,
      links,
      args.context ?? "",
      args.marketingPurpose ?? "",
    ],
    enabled,
    // A refusal is live state — somebody may unsubscribe between the preview
    // and the send — so this is a reading taken close to the write, never a
    // cached verdict that outlives what it described.
    staleTime: 0,
    queryFn: async () => {
      const body = {
        to: [...recipients],
        communication_context: args.context,
        marketing_purpose: args.marketingPurpose,
      };
      const answered = args.anchorActivityId
        ? await api.POST("/activities/{id}/send-email:preview", {
            params: { path: { id: args.anchorActivityId } },
            body,
          })
        : await api.POST("/emails:preview", {
            body: { ...body, links: [...links] },
          });
      // Only a real 2xx is an answer. openapi-fetch reports a falsy `error` for
      // a bodiless non-2xx — an empty 502 from a gateway — and taking that as
      // success would resolve with no preview, which renders as ALLOWED: a
      // permission check that failed, drawn as permission granted.
      if (!answered.response.ok) {
        throwProblem(
          answered.error ?? {
            title: `send permission: the preview answered ${answered.response.status}`,
          },
        );
      }
      return answered.data;
    },
  });

  // A preview that FAILED is not a refusal, and must never render as one: the
  // engine did not say no, the question did not arrive. `unanswered` lets a
  // surface say so rather than fall silent, because silence is indistinguishable
  // from an allow — which is how a rep learns of a refusal by pressing Send,
  // the exact failure this hook exists to end.
  return {
    preview: query.data,
    asking: query.isPending && enabled,
    unanswered: enabled && query.isError,
  };
}
