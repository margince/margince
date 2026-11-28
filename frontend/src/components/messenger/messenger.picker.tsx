// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useInfiniteQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Checkbox, SearchField } from "../../design-system/field";
import { QueryStates } from "../../design-system/querystates";
import { useT } from "../../i18n";
import { useCanMutate } from "../../lib/capability";
import { problemMessageOf, unwrap } from "../../lib/problem";
import {
  type MessengerMember,
  type MessengerScope,
  messengerPath,
  useMessengerOpenPeople,
} from "./messenger.queries";
import { MessengerTeamPicker } from "./messenger.team";

function DirectoryPicker({
  scope,
  onChoose,
  mode,
}: Readonly<{
  scope: MessengerScope;
  onChoose: (id: string) => void;
  mode: "channels" | "members";
}>) {
  const t = useT();
  const canMutate = useCanMutate();
  const [search, Search] = useState("");
  const [selected, Selected] = useState<MessengerMember[]>([]);
  const channels = useInfiniteQuery({
    queryKey: ["messenger-channels", scope],
    initialPageParam: "",
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api.GET("/messenger/{provider}/{record_type}/{id}/channels", {
          params: {
            ...messengerPath(scope),
            query: { cursor: pageParam || undefined },
          },
        }),
      ),
    getNextPageParam: (page) => page.next_cursor || undefined,
    enabled: mode === "channels",
    retry: false,
  });
  const members = useInfiniteQuery({
    queryKey: ["messenger-members", scope, search],
    initialPageParam: "",
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api.GET("/messenger/{provider}/{record_type}/{id}/members", {
          params: {
            ...messengerPath(scope),
            query: { q: search, cursor: pageParam || undefined },
          },
        }),
      ),
    getNextPageParam: (page) => page.next_cursor || undefined,
    enabled: mode === "members",
    retry: false,
  });
  const open = useMessengerOpenPeople();
  const query = mode === "channels" ? channels : members;
  return (
    <div className="messenger-picker">
      {mode === "members" && (
        <SearchField
          aria-label={t("messenger.searchPeople")}
          value={search}
          onChange={(event) => Search(event.target.value)}
        />
      )}
      <QueryStates query={query} pendingLabel={t("messenger.loading")}>
        <div className="messenger-destinations">
          {mode === "channels"
            ? channels.data?.pages
                .flatMap((page) => page.conversations)
                .map((channel) => (
                  <Button
                    align="start"
                    key={channel.id}
                    onClick={() => onChoose(channel.id)}
                  >
                    {channel.name}
                  </Button>
                ))
            : members.data?.pages
                .flatMap((page) => page.members)
                .map((person) => (
                  <Checkbox
                    key={person.id}
                    label={person.name}
                    checked={selected.some((item) => item.id === person.id)}
                    disabled={!person.active || person.external}
                    onChange={(event) =>
                      Selected(
                        event.target.checked
                          ? [...selected, person]
                          : selected.filter((item) => item.id !== person.id),
                      )
                    }
                  />
                ))}
        </div>
        {query.hasNextPage && (
          <Button
            pending={query.isFetchingNextPage}
            onClick={() => {
              void query.fetchNextPage();
            }}
          >
            {t("messenger.more")}
          </Button>
        )}
      </QueryStates>
      {mode === "members" && (
        <>
          <p>
            {t("messenger.audience", {
              names: selected.map((person) => person.name).join(", "),
            })}
          </p>
          <Button
            variant="primary"
            pending={open.isPending}
            disabled={!canMutate || selected.length === 0}
            reason={
              !canMutate
                ? t("settings.seatRefused")
                : selected.length === 0
                  ? t("messenger.recipientsRequired")
                  : undefined
            }
            onClick={() =>
              open.mutate(
                { scope, members: selected.map((person) => person.id) },
                { onSuccess: (context) => onChoose(context.conversation.id) },
              )
            }
          >
            {t("messenger.openPeople")}
          </Button>
        </>
      )}
      {open.error && (
        <Callout tone="warning" title={t("messenger.loadProblem")}>
          {problemMessageOf(open.error, t)}
        </Callout>
      )}
    </div>
  );
}

export function MessengerPicker({
  scope,
  onChoose,
}: Readonly<{ scope: MessengerScope; onChoose: (id: string) => void }>) {
  const t = useT();
  const [mode, Mode] = useState<"team" | "channels" | "members">("team");
  return (
    <div className="messenger-picker">
      <div className="messenger-choices">
        <Button aria-pressed={mode === "team"} onClick={() => Mode("team")}>
          {t("messenger.team")}
        </Button>
        <Button
          aria-pressed={mode === "channels"}
          onClick={() => Mode("channels")}
        >
          {t("messenger.channels")}
        </Button>
        <Button
          aria-pressed={mode === "members"}
          onClick={() => Mode("members")}
        >
          {t("messenger.people")}
        </Button>
      </div>
      {mode === "team" ? (
        <MessengerTeamPicker scope={scope} onChoose={onChoose} />
      ) : (
        <DirectoryPicker scope={scope} mode={mode} onChoose={onChoose} />
      )}
    </div>
  );
}
