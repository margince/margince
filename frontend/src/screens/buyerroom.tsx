import { useMutation, useQuery } from "@tanstack/react-query";
import { LogOut } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { forgetHashCredential, takeHashCredential } from "../app/router";
import { Button, EmptyState } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { BuyerBoard } from "./buyerroomboard";
import {
  BuyerFrame,
  BuyerHero,
  ContactCard,
  DeadLink,
  LinkRequest,
  stewardLabel,
} from "./buyerroomframe";
import {
  bearer,
  readSession,
  SessionRefusedError,
  writeSession,
} from "./buyerroomsession";
import { problemMessageOf, QueryStates, throwProblem } from "./common";
import "./buyerroom.css";

// The Deal Room as its BUYER sees it — the one screen an outside contact ever
// reaches in this app, drawn like the front of a small, well-made site: a
// hero, the documents as a gallery with the conversation under each, and the
// contact beside them. Anonymous: no seat, no cookie. The invitation link lands
// on `#/room?c=<credential>`; the router takes the credential off the hash
// (app/router.tsx's takeHashCredential, ahead of every gate that can render
// instead of this screen) and this screen exchanges it for a room session the
// tab keeps in sessionStorage and presents as a Bearer on every call. A dead
// link, a paused room and an expired one each get an honest screen and a way
// back, none naming anything the link did not.

type BuyerRoomView = components["schemas"]["BuyerRoomView"];

const ROOM_ROUTE = "room";

export function BuyerRoomScreen() {
  // Read at mount AND whenever the address changes to carry a new one.
  //
  // A SECOND link pasted into a tab already on #/room changes only the hash,
  // which React does not treat as a new mount. Read once, that link is ignored
  // and the tab keeps whatever session it holds, including a dead one: the
  // buyer sees "Nothing published yet" and concludes the link is broken.
  const [credential, setCredential] = useState(() =>
    takeHashCredential(ROOM_ROUTE),
  );
  useEffect(() => {
    const onHashChange = () => {
      const next = takeHashCredential(ROOM_ROUTE);
      if (next) {
        setCredential(next);
      }
    };
    globalThis.addEventListener?.("hashchange", onHashChange);
    return () => globalThis.removeEventListener?.("hashchange", onHashChange);
  }, []);
  // A link in hand outranks a kept session from the first render: a tab that
  // still holds room A's session must not show room A for a breath while
  // room B's link is being exchanged.
  const [token, setToken] = useState(() => (credential ? null : readSession()));
  // What the exchange answered, held HERE rather than read off the mutation:
  // the replayed mount (StrictMode) swaps the observer that ran it for one that
  // never hears the result, so isSuccess/isError would stay false for ever.
  const [refusal, setRefusal] = useState<Error | null>(null);
  const t = useT();

  const exchange = useMutation({
    mutationKey: ["buyer-room-exchange"],
    mutationFn: async (raw: string) => {
      const { data, error, response } = await api.POST(
        "/public/rooms/exchange",
        { body: { credential: raw } },
      );
      if (error) {
        if (response.status === 404) {
          throw new SessionRefusedError();
        }
        throwProblem(error, t);
      }
      return data;
    },
  });

  // A fresh link outranks a kept session: the contact clicked it on purpose.
  // Exchanged at most ONCE per mount, held in a ref rather than in state: the
  // credential is single-use, and an effect that runs twice (StrictMode replays
  // mount effects in development) would spend it on the first run and be
  // refused on the second, showing a dead-link page for a live link. The token
  // comes off the promise rather than an onSuccess option, because the replayed
  // mount unsubscribes the first observer and an option callback on an observer
  // nobody listens to never runs.
  const exchangeAsync = exchange.mutateAsync;
  // Every credential this tab has ALREADY spent, not merely the last one. A
  // link is single-use, so A → B → A must not send A twice: the server refuses
  // the replay, and that refusal would displace the session B just opened.
  const spent = useRef(new Set<string>());
  // Which credential the tab is currently exchanging. A reply for anything else
  // is a superseded link answering late and must not touch the session — two
  // links pasted in quick succession would race, and whichever answered last
  // would win regardless of which the contact meant.
  const awaiting = useRef<string | null>(null);
  useEffect(() => {
    if (!credential || spent.current.has(credential)) {
      return;
    }
    spent.current.add(credential);
    // Out of the router's memory as well: this tab is spending it now, and a
    // remount that found it there would spend it again and be refused.
    forgetHashCredential(ROOM_ROUTE, credential);
    awaiting.current = credential;
    // The session the tab already holds is KEPT while the new link is checked.
    // Cleared first, an expired new link drew the dead-link page over a room
    // the contact could still read, and a refresh brought it back from storage —
    // two answers to one question a moment apart.
    setRefusal(null);
    exchangeAsync(credential).then(
      (issued) => {
        if (awaiting.current !== credential || !issued) {
          return;
        }
        awaiting.current = null;
        writeSession(issued.session_token);
        setToken(issued.session_token);
      },
      (error: unknown) => {
        if (awaiting.current !== credential) {
          return;
        }
        awaiting.current = null;
        setRefusal(error instanceof Error ? error : new Error(String(error)));
      },
    );
  }, [credential, exchangeAsync]);

  const signOut = () => {
    writeSession(null);
    setToken(null);
  };

  // "Opening" until the exchange has answered — not merely while the mutation
  // is in flight, because the first render happens before the effect fires it.
  if (credential && !token && !refusal) {
    return (
      <BuyerFrame>
        <EmptyState>{t("buyer.opening")}</EmptyState>
      </BuyerFrame>
    );
  }
  if (credential && refusal) {
    return (
      <BuyerFrame>
        <DeadLink
          message={
            refusal instanceof SessionRefusedError
              ? t("buyer.linkDead")
              : problemMessageOf(refusal, t)
          }
        />
      </BuyerFrame>
    );
  }
  if (!token) {
    return (
      <BuyerFrame>
        <DeadLink message={t("buyer.noLink")} />
      </BuyerFrame>
    );
  }
  return (
    <BuyerFrame>
      <RoomBody token={token} onSessionLost={signOut} />
    </BuyerFrame>
  );
}

function useBuyerRoom(token: string, onSessionLost: () => void) {
  const t = useT();
  const query = useQuery({
    queryKey: ["buyer-room", token],
    retry: false,
    // Re-asked whenever the tab comes back: a revocation or a pause made while
    // the buyer was away must bind on their return, not on their next click.
    refetchOnWindowFocus: "always",
    queryFn: async () => {
      const { data, error, response } = await api.GET("/public/rooms/me", {
        ...bearer(token),
      });
      if (error) {
        if (response.status === 401) {
          throw new SessionRefusedError();
        }
        throwProblem(error, t);
      }
      return data;
    },
  });
  const lost = query.error instanceof SessionRefusedError;
  useEffect(() => {
    if (lost) {
      onSessionLost();
    }
  }, [lost, onSessionLost]);
  return query;
}

function RoomBody({
  token,
  onSessionLost,
}: Readonly<{ token: string; onSessionLost: () => void }>) {
  const t = useT();
  const room = useBuyerRoom(token, onSessionLost);
  const signOut = useMutation({
    mutationKey: ["buyer-room-sign-out"],
    mutationFn: async (session: string) => {
      const { error } = await api.POST("/public/rooms/sign-out", {
        ...bearer(session),
      });
      if (error) {
        throwProblem(error, t);
      }
    },
    // Whatever the server said, this tab is done with the token.
    onSettled: onSessionLost,
  });
  return (
    <QueryStates
      query={room}
      pendingLines={4}
      pendingLabel={t("room.card.title")}
    >
      {room.data ? (
        <>
          <RoomView
            view={room.data}
            token={token}
            onSessionLost={onSessionLost}
          />
          <div className="buyer-foot">
            <p className="t-caption">
              {t("buyer.signedInAs", {
                name: room.data.participant.full_name,
              })}
            </p>
            <Button
              variant="ghost"
              pending={signOut.isPending}
              onClick={() => signOut.mutate(token)}
            >
              <LogOut aria-hidden />
              {t("buyer.signOut")}
            </Button>
          </div>
        </>
      ) : null}
    </QueryStates>
  );
}

const ACCESS_TITLE: Record<string, MessageKey> = {
  paused: "buyer.pausedTitle",
  expired: "buyer.expiredTitle",
};

// Why this reader may not write in the conversation, in the order that binds
// first: a preview never writes, a room that is not open takes nothing more, a
// read-only seat may only read. Undefined when they may.
//
// The access test names the ONE state that admits a write rather than the
// states that refuse one. `BuyerRoomAccess` is a plain string on the wire, not
// a union, so a fifth state added on the server reaches this untouched —
// listing the refusals would let it arrive writable, the wrong way for a write
// gate to be wrong. `paused` and `expired` do not reach this code today
// (RoomView answers them first), and this does not rely on that.
function conversationRefusal(
  view: BuyerRoomView,
  t: ReturnType<typeof useT>,
): string | undefined {
  if (view.preview) {
    return t("buyer.previewReadOnly");
  }
  if (view.access === "closed") {
    return t("buyer.closed");
  }
  if (view.access !== "live") {
    // Any other non-live state, including one this build has never heard of.
    // `buyer.closedNote` rather than `buyer.closed`: "this room is closed" is
    // a claim, and it is false for a paused room.
    return t("buyer.closedNote");
  }
  if (view.participant.capability === "view") {
    return t("threads.readOnly");
  }
  return undefined;
}

/**
 * The one preview banner: `RoomView` returns from two branches — the closed
 * room and the live one — and a copy in each is two places to change.
 */
function PreviewBanner() {
  const t = useT();
  return (
    <Callout tone="info" kind="standing" title={t("buyer.previewBannerTitle")}>
      {t("buyer.previewBanner")}
    </Callout>
  );
}

function RoomView({
  view,
  token,
  onSessionLost,
}: Readonly<{
  view: BuyerRoomView;
  token: string;
  onSessionLost: () => void;
}>) {
  const t = useT();
  const steward = stewardLabel(view.steward_name, t);
  // Whether this reader may write is the ANSWER to `conversationRefusal`, not
  // a second opinion beside it: a reader given a reason may not write, and one
  // who may write has no reason to show. Spelled apart the two drifted, and a
  // preview seat carrying `comment` would have been handed a live composer —
  // which does not arise today only because every preview seat is read-only.
  const writeRefusal = conversationRefusal(view, t);
  if (view.access === "paused" || view.access === "expired") {
    return (
      <>
        {view.preview ? <PreviewBanner /> : null}
        <Panel title={t(ACCESS_TITLE[view.access])}>
          <PanelBody>
            <p>
              {t(
                view.access === "paused"
                  ? "buyer.pausedBody"
                  : "buyer.expiredBody",
                { steward },
              )}
            </p>
          </PanelBody>
          {view.access === "expired" ? (
            <PanelBody>
              <LinkRequest />
            </PanelBody>
          ) : null}
        </Panel>
      </>
    );
  }
  if (!view.room) {
    // A seat that resolves always has a room to read: the server withholds
    // content only in the paused and expired states, both handled above. This
    // is the impossible branch, and it says so rather than rendering a blank.
    return (
      <Panel title={t("buyer.deadTitle")}>
        <PanelBody>
          <p>{t("buyer.deadAskContact")}</p>
        </PanelBody>
      </Panel>
    );
  }
  return (
    <>
      {view.preview ? <PreviewBanner /> : null}
      <BuyerHero
        title={view.room.title}
        welcome={view.room.welcome_message ?? ""}
        access={view.access}
        closedAt={view.room.closed_at}
      />
      <div className="buyer-grid">
        <div className="buyer-main">
          <BuyerBoard
            token={token}
            onSessionLost={onSessionLost}
            mayWrite={writeRefusal === undefined}
            refusal={writeRefusal}
          />
        </div>
        <aside className="buyer-side">
          <ContactCard stewardName={view.steward_name} access={view.access} />
        </aside>
      </div>
    </>
  );
}
