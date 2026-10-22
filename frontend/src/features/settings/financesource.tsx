import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect, useId, useRef, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Badge, Button, EmptyState } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { DataTable } from "../../design-system/datatable";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { LoadMoreButton, QueryGate } from "../../design-system/querystates";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../../design-system/recordpicker";
import { Select } from "../../design-system/select";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { useToast } from "../../design-system/toast";
import { formatDateTime } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { problemMessageOf, throwProblem, unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";
import { searchCompanyCandidates } from "../contacts/contactemployers";

// Settings → Integrations: the accounting source the finance card reads. The
// core ships none; every source on offer is an extension unit. Margince only
// reads it, so nothing here can write to the accounting system.

type FinanceSourceState = components["schemas"]["FinanceSourceState"];
type FinanceConnection = components["schemas"]["FinanceConnection"];
type FinanceSourceCustomer = components["schemas"]["FinanceSourceCustomer"];

const SOURCE_KEY = ["finance-source"] as const;
const CUSTOMERS_KEY = ["finance-source", "customers"] as const;
const SYNC_POLL_MS = 3_000;

const STATUS_TONE = {
  connecting: "info",
  active: "success",
  error: "danger",
} as const;

// A connect or a sync only queues the job, so the card polls until the job
// records an attempt. `awaiting` holds the attempt time seen when it queued.
function useFinanceSource(awaiting: string | null | undefined) {
  return useQuery({
    queryKey: SOURCE_KEY,
    queryFn: async () => unwrap(await api.GET("/finance/source")),
    refetchInterval: (query) => {
      const connection = query.state.data?.connection;
      const pending =
        connection?.status === "connecting" ||
        (awaiting !== undefined &&
          (connection?.last_attempt_at ?? null) === awaiting);
      return pending ? SYNC_POLL_MS : false;
    },
  });
}

export function FinanceSourceCard() {
  const t = useT();
  const queryClient = useQueryClient();
  const [awaiting, setAwaiting] = useState<string | null | undefined>();
  const query = useFinanceSource(awaiting);
  const me = useMe();
  const canConnect = useCanWrite("integrations", "create");
  const canSync = useCanWrite("integrations", "update");
  const canDisconnect = useCanWrite("integrations", "delete");
  // Gated on /me having answered, so a reader who holds a grant never sees
  // the line flash while it loads.
  const readOnly = me.isSuccess && !canConnect && !canSync && !canDisconnect;
  const attempt = query.data?.connection?.last_attempt_at;
  const seenAttempt = useRef(attempt);
  // A finished attempt may have filled the directory, so it is read again.
  useEffect(() => {
    if (seenAttempt.current === attempt) return;
    seenAttempt.current = attempt;
    setAwaiting(undefined);
    void queryClient.invalidateQueries({ queryKey: CUSTOMERS_KEY });
  }, [attempt, queryClient]);
  return (
    <Panel title={t("financeSource.title")}>
      <PanelBody>
        <PanelIntro>{t("financeSource.sub")}</PanelIntro>
        {readOnly && <PanelIntro>{t("financeSource.readOnly")}</PanelIntro>}
        <QueryGate pendingLabel={t("financeSource.title")} query={query}>
          {(state) =>
            state.connection ? (
              <>
                <ConnectedSource
                  connection={state.connection}
                  onSyncQueued={() =>
                    setAwaiting(state.connection?.last_attempt_at ?? null)
                  }
                />
                <CustomerDirectory connectionId={state.connection.id} />
              </>
            ) : (
              <ConnectSource state={state} />
            )
          }
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}

function ConnectSource({ state }: Readonly<{ state: FinanceSourceState }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const canConnect = useCanWrite("integrations", "create");
  const [provider, setProvider] = useState("");
  const connect = useMutation({
    mutationFn: async (chosen: string) =>
      unwrap(await api.PUT("/finance/source", { body: { provider: chosen } })),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: SOURCE_KEY });
    },
  });

  if (state.available.length === 0) {
    return (
      <EmptyState title={t("financeSource.noneInstalled")}>
        {t("financeSource.noneInstalledBody")}
      </EmptyState>
    );
  }
  if (!canConnect) return null;
  const chosen = provider || (state.available[0]?.provider ?? "");
  const description = state.available.find(
    (option) => option.provider === chosen,
  )?.description;
  return (
    <SettingList>
      <SettingRow
        label={t("financeSource.pick")}
        description={description}
        control={(control) => (
          <div className="card-actions">
            <Select
              {...control}
              options={state.available.map((option) => ({
                value: option.provider,
                label: option.provider,
              }))}
              value={chosen}
              onChange={setProvider}
              disabled={connect.isPending}
            />
            <Button
              variant="primary"
              data-testid="finance-source-connect"
              pending={connect.isPending}
              onClick={() => connect.mutate(chosen)}
            >
              {t("financeSource.connect")}
            </Button>
          </div>
        )}
      />
      <ErrorLine error={connect.error} />
    </SettingList>
  );
}

function ConnectedSource({
  connection,
  onSyncQueued,
}: Readonly<{ connection: FinanceConnection; onSyncQueued: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const toast = useToast();
  const canSync = useCanWrite("integrations", "update");
  const canDisconnect = useCanWrite("integrations", "delete");
  const [confirming, setConfirming] = useState(false);
  const sync = useSyncNow();
  const disconnect = useMutation({
    mutationFn: async () => {
      const { error, response } = await api.DELETE("/finance/source");
      if (error || !response.ok) throwProblem(error);
    },
    onSuccess: async () => {
      setConfirming(false);
      await queryClient.invalidateQueries({ queryKey: SOURCE_KEY });
    },
  });

  const lastSync = connection.last_success_at
    ? t("financeSource.lastSync", {
        date: formatDateTime(connection.last_success_at, locale, viewerZone()),
      })
    : t("financeSource.notSynced");
  return (
    <SettingList>
      <SettingRow
        label={connection.provider}
        description={lastSync}
        value={
          <Badge tone={STATUS_TONE[connection.status]}>
            {t(`financeSource.status.${connection.status}`)}
          </Badge>
        }
        control={
          <div className="card-actions">
            {canSync && (
              <Button
                data-testid="finance-source-sync"
                pending={sync.isPending}
                onClick={() =>
                  sync.mutate(undefined, {
                    onSuccess: () => {
                      onSyncQueued();
                      toast.show(t("financeSource.syncQueued"));
                    },
                  })
                }
              >
                {t("financeSource.syncNow")}
              </Button>
            )}
            {canDisconnect && (
              <Button
                variant="ghost"
                data-testid="finance-source-disconnect"
                onClick={() => setConfirming(true)}
              >
                {t("financeSource.disconnect")}
              </Button>
            )}
          </div>
        }
      />
      <ErrorLine error={sync.error} />
      <ConfirmModal
        open={confirming}
        onClose={() => setConfirming(false)}
        title={t("financeSource.disconnectTitle")}
        confirmLabel={t("financeSource.disconnect")}
        confirmVariant="danger"
        pending={disconnect.isPending}
        error={disconnect.error ? problemMessageOf(disconnect.error, t) : null}
        onConfirm={() => disconnect.mutate()}
      >
        <p>{t("financeSource.disconnectBody")}</p>
      </ConfirmModal>
    </SettingList>
  );
}

function useSyncNow() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => unwrap(await api.POST("/finance/source/sync")),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: SOURCE_KEY });
    },
  });
}

// Keyed by connection, so a source connected after another never shows the
// earlier source's customers while its own load.
function useCustomers(connectionId: string) {
  return useInfiniteQuery({
    queryKey: [...CUSTOMERS_KEY, connectionId],
    initialPageParam: null as string | null,
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api.GET("/finance/source/customers", {
          params: { query: { cursor: pageParam ?? undefined } },
        }),
      ),
    getNextPageParam: (last) => last.page.next_cursor ?? null,
  });
}

function CustomerDirectory({
  connectionId,
}: Readonly<{ connectionId: string }>) {
  const t = useT();
  const query = useCustomers(connectionId);
  const canLink = useCanWrite("integrations", "update");
  const [linking, setLinking] = useState<FinanceSourceCustomer | null>(null);
  const unlink = useLinkWrite();

  return (
    <>
      <Heading size="xsmall" as="h3">
        {t("financeSource.customers")}
      </Heading>
      <QueryGate pendingLabel={t("financeSource.customers")} query={query}>
        {(data) => {
          const rows = data.pages.flatMap((page) => page.data);
          if (rows.length === 0) {
            return (
              <EmptyState title={t("financeSource.noCustomers")}>
                {t("financeSource.noCustomersBody")}
              </EmptyState>
            );
          }
          return (
            <>
              <DataTable
                label={t("financeSource.customers")}
                rows={rows}
                rowKey={(row) => row.external_id}
                rowTestId={(row) => `finance-customer-${row.external_id}`}
                columns={[
                  {
                    key: "customer",
                    header: t("financeSource.customer"),
                    render: (row) => (
                      <>
                        {row.display_name}
                        <span className="t-caption"> · {row.external_id}</span>
                      </>
                    ),
                  },
                  {
                    key: "company",
                    header: t("financeSource.company"),
                    render: (row) =>
                      row.company?.display_name ?? t("financeSource.notLinked"),
                  },
                  {
                    key: "actions",
                    header: t("financeSource.actions"),
                    headerHidden: true,
                    render: (row) =>
                      canLink && (
                        <div className="cell-actions">
                          <Button
                            variant="ghost"
                            onClick={() => setLinking(row)}
                          >
                            {row.company
                              ? t("financeSource.relink")
                              : t("financeSource.link")}
                          </Button>
                          {row.company && (
                            <Button
                              variant="ghost"
                              pending={
                                unlink.isPending &&
                                unlink.variables?.customer.external_id ===
                                  row.external_id
                              }
                              onClick={() =>
                                unlink.mutate({ customer: row, company: null })
                              }
                            >
                              {t("financeSource.unlink")}
                            </Button>
                          )}
                        </div>
                      ),
                  },
                ]}
              />
              <LoadMoreButton query={query} />
            </>
          );
        }}
      </QueryGate>
      <ErrorLine error={unlink.error} />
      {linking && (
        <LinkCustomerModal
          customer={linking}
          onClose={() => setLinking(null)}
        />
      )}
    </>
  );
}

type LinkWrite = Readonly<{
  customer: FinanceSourceCustomer;
  company: RecordPickerCandidate | null;
}>;

// One mutation for both directions: a company links the customer to it, no
// company removes the link. Either way the directory and the card re-read.
function useLinkWrite() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ customer, company }: LinkWrite) => {
      const path = {
        params: { path: { external_id: customer.external_id } },
      };
      const { error, response } = company
        ? await api.PUT("/finance/source/customers/{external_id}/link", {
            ...path,
            body: { company_id: company.id },
          })
        : await api.DELETE(
            "/finance/source/customers/{external_id}/link",
            path,
          );
      if (error || !response.ok) throwProblem(error);
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: CUSTOMERS_KEY });
    },
  });
}

function LinkCustomerModal({
  customer,
  onClose,
}: Readonly<{ customer: FinanceSourceCustomer; onClose: () => void }>) {
  const t = useT();
  const headingId = useId();
  const [company, setCompany] = useState<RecordPickerCandidate | null>(null);
  const link = useLinkWrite();
  return (
    <Modal
      open
      onClose={() => {
        if (!link.isPending) onClose();
      }}
      labelledBy={headingId}
      intent="form"
    >
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {t("financeSource.linkTitle", { customer: customer.display_name })}
      </Heading>
      <div className="form-stack">
        <RecordPicker
          label={t("financeSource.findCompany")}
          searchTargets={searchCompanyCandidates}
          selected={company}
          onPick={setCompany}
          disabled={link.isPending}
          initialTerm={customer.display_name}
        />
        <ErrorLine error={link.error} />
      </div>
      <div className="actions">
        <Button variant="ghost" onClick={onClose} disabled={link.isPending}>
          {t("financeSource.cancel")}
        </Button>
        <Button
          variant="primary"
          disabled={!company}
          pending={link.isPending}
          onClick={() =>
            company &&
            link.mutate({ customer, company }, { onSuccess: onClose })
          }
        >
          {t("financeSource.link")}
        </Button>
      </div>
    </Modal>
  );
}
