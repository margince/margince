import { useInfiniteQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { LogActivityForm } from "../../components/logactivity";
import { Badge, Button } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { Row, Stack } from "../../design-system/stack";
import {
  type SectionState,
  SurfaceState,
} from "../../design-system/surfacestate";
import { formatDate } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { activityFollowUpsKey } from "../../lib/activitykeys";
import { useCanWrite } from "../../lib/capability";
import { problemCodeOf, unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";

export function ActivityFollowUps({
  sourceId,
  archived,
  onOpen,
}: Readonly<{
  sourceId: string;
  archived: boolean;
  onOpen: (id: string) => void;
}>) {
  const t = useT();
  const canCreate = useCanWrite("activity", "create");
  const [adding, setAdding] = useState(false);
  const query = useInfiniteQuery({
    queryKey: activityFollowUpsKey(sourceId),
    initialPageParam: undefined,
    staleTime: 0,
    gcTime: 0,
    queryFn: async ({ pageParam }: { pageParam: string | undefined }) =>
      unwrap(
        await api.GET("/activities", {
          params: {
            query: {
              source_activity_id: sourceId,
              cursor: pageParam,
              limit: 20,
            },
          },
        }),
        t,
      ),
    getNextPageParam: (page) =>
      page.page.has_more ? (page.page.next_cursor ?? undefined) : undefined,
  });
  const tasks = query.isError
    ? []
    : (query.data?.pages.flatMap((page) => page.data) ?? []);
  const unavailable = ["permission_denied", "not_found"].includes(
    problemCodeOf(query.error) ?? "",
  );
  return (
    <Panel title={t("activity.followUps")}>
      <PanelBody>
        <Stack>
          <SurfaceState
            state={followUpState(
              unavailable,
              query.isError,
              query.isPending,
              tasks.length,
            )}
            emptyLabel={t("activity.noFollowUps")}
            loadingLabel={t("activity.detail.loading")}
            detail={
              unavailable
                ? { withheldReason: t("activity.detail.withheldReason") }
                : { onRetry: () => void query.refetch() }
            }
          >
            <Stack gap="2">
              {tasks.map((task) => (
                <FollowUpRow key={task.id} task={task} onOpen={onOpen} />
              ))}
            </Stack>
          </SurfaceState>
          {query.hasNextPage && !query.isError && (
            <Button
              variant="ghost"
              pending={query.isFetchingNextPage}
              onClick={() => void query.fetchNextPage()}
            >
              {t("list.loadMore")}
            </Button>
          )}
          {!archived &&
            !unavailable &&
            canCreate &&
            (adding ? (
              <>
                <LogActivityForm
                  sourceActivityId={sourceId}
                  onlyTasks
                  askedKind="task"
                  onLogged={() => setAdding(false)}
                />
                <Button variant="ghost" onClick={() => setAdding(false)}>
                  {t("create.cancel")}
                </Button>
              </>
            ) : (
              <Button variant="ghost" onClick={() => setAdding(true)}>
                {t("log.addFollowUp")}
              </Button>
            ))}
        </Stack>
      </PanelBody>
    </Panel>
  );
}

function FollowUpRow({
  task,
  onOpen,
}: Readonly<{
  task: components["schemas"]["Activity"];
  onOpen: (id: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  return (
    <Stack gap="1">
      <Row justify="between">
        <Button variant="link" onClick={() => onOpen(task.id)}>
          {task.content_state === "withheld"
            ? t("timeline.withheld")
            : task.subject || t("timeline.kind.task")}
        </Button>
        <Badge tone={task.is_done ? "success" : "default"}>
          {t(task.is_done ? "tasks.isDone" : "activity.followUpOpen")}
        </Badge>
      </Row>
      <p className="t-caption">
        {task.assignee_id ? (
          <EntityRef kind="user" id={task.assignee_id} />
        ) : (
          t("log.unassigned")
        )}
        {task.due_at && (
          <>
            {" "}
            ·{" "}
            {t("co.next.due", { when: formatDate(task.due_at, locale, zone) })}
          </>
        )}
      </p>
    </Stack>
  );
}

function followUpState(
  unavailable: boolean,
  failed: boolean,
  pending: boolean,
  count: number,
): SectionState {
  if (unavailable) return "withheld";
  if (failed) return "failed";
  if (pending) return "loading";
  return count > 0 ? "ready" : "empty";
}
