import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

type Person360 = components["schemas"]["Person360"];

// Which addresses a message would not arrive at, and whether a channel reply
// can be offered at all. Extracted from compose.tsx unchanged.
//
// Both answers come off the person's 360, which is why they belong together:
// the composer holds that payload already, and two hooks reaching for the same
// view would ask for it twice.

// WHICH OF THESE ADDRESSES IS KNOWN NOT TO ARRIVE. The person page badges an
// address whose latest delivery hard-bounced with nothing clean since, and the
// composer is where that matters: the mark was visible only on a page the rep
// is not looking at while they write.
//
// A function of the 360 the drawer is already holding, rather than a read of its
// own. That payload comes back under the SAME key the person page fetches under,
// so opening the composer from that page costs no request at all and the two
// surfaces cannot disagree about which address is dead — and it now answers a
// second question beside this one (who the recipient fields offer), which two
// hooks reaching for the same view would have asked twice. A composer that never
// learned a person — a deal timeline has no single one — is handed nothing and
// warns about nothing.
//
// The section carries its own grant. A caller who may not read the send ledger
// gets it omitted rather than empty, and this then marks nothing: an unanswered
// read is not "the address is fine", and a warning invented from an absence
// would be a claim about correspondence the reader may not see.
//
// Free-typed addresses with no person context stay unwarned on purpose (#3160):
// deriving deadness for an arbitrary string needs an endpoint of its own.
export function deadRecipientsAmong(
  view: Person360 | undefined,
  recipients: readonly string[],
) {
  const dead = view?.dead_addresses;
  if (dead == null || dead.length === 0) return [];
  // Addresses compare case-insensitively — a rep who types Anna@… must be
  // warned about anna@…, and the ledger stores what the provider reported.
  const marked = new Set(dead.map((address) => address.toLowerCase()));
  // ONE MENTION PER ADDRESS. To and Cc are asked about together, and a rep who
  // has the same address in both would otherwise read it named twice in a
  // sentence about one thing being wrong with it.
  const named = new Map<string, string>();
  for (const address of recipients) {
    const key = address.toLowerCase();
    if (marked.has(key) && !named.has(key)) {
      named.set(key, address);
    }
  }
  return [...named.values()];
}

// A channel reply can only land on a live, unblocked identity, and the
// failure otherwise arrives after the rep has already written the message —
// worse than never offering the box (design §9.3). Reachability is read off
// the person the row's own timeline names: `["person", personId]` is the same
// query key the 360 screen already fetches under, so this rides its cache
// instead of opening a second request. A caller that never learned a personId
// (e.g. a deal timeline, which has no single person to check) gets the
// pre-existing behaviour of always offering the reply — this only ever turns
// the action OFF, never on, for a row it cannot verify.
export function useChannelReachable(
  isChannel: boolean,
  personId: string | undefined,
  provider: string | undefined,
) {
  const person = useQuery({
    queryKey: ["person", personId],
    queryFn: async () => {
      const { data, error } = await api.GET("/people/{id}", {
        params: { path: { id: personId as string } },
      });
      if (error) throwProblem(error);
      return data;
    },
    enabled: isChannel && personId != null,
  });
  if (!isChannel || personId == null) return true;
  // Matched against the row's OWN transport. A hardcoded "telegram" here would
  // withhold the reply on every other transport's rows and offer it on a
  // Telegram-reachable person's rows whatever carried the conversation — the
  // activity kind no longer names the transport — a `message` row can have been
  // carried by any provider — so only the row itself can say which one this
  // conversation used (ADR-0107).
  return (person.data?.reachability ?? []).some(
    (channel) => channel.provider === provider && channel.reachable,
  );
}
