import { api, QueryStates, throwProblem } from "@margince/frontend/api";
import { formatNumber, useCanWrite, useLocale, useT } from "@margince/frontend/app";
import {
  Badge,
  Button,
  Card,
  DataTable,
  ErrorLine,
  Row,
  SectionHeader,
  Stack,
  TextInput,
} from "@margince/frontend/design-system";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

// #/ext/billing-demo: the demonstration billing system, driven from one screen.
//
// An admin connects it, marks an invoice paid, or changes its amount. The unit
// offers the change to the core on its next tick, so a row reads "waiting"
// until the core has the latest offer.

const FEED_OBJECT = "ext_billing_demo_feed";
const STATUS_KEY = ["ext", "billing-demo", "status"];
const SETUP_KEY = ["ext", "billing-demo", "setup"];
const POLL_MS = 5_000;

type Invoice = {
  erp_id: string;
  name: string;
  currency: string;
  amount_minor: number;
  paid: boolean;
  synced: boolean;
  deal_id: string;
  withheld: string[];
};

/** What the connection needs, found among the core's own records. */
type Setup = {
  member_id: string;
  pipeline_id: string;
  open_stage_id: string;
  won_stage_id: string;
};

/**
 * The service member, the default pipeline, its first open stage and its won
 * stage, read from the core so nobody types an id. Null when one is missing.
 */
async function findSetup(): Promise<Setup | null> {
  const users = await api.GET("/users");
  const pipelines = await api.GET("/pipelines");
  if (users.error || !users.response.ok) {
    throwProblem(users.error);
  }
  if (pipelines.error || !pipelines.response.ok) {
    throwProblem(pipelines.error);
  }
  const member = users.data?.data.find(
    (user) => user.service_extension === "billing-demo",
  );
  const pipeline =
    pipelines.data?.data.find((candidate) => candidate.is_default) ??
    pipelines.data?.data[0];
  const open = pipeline?.stages?.find((stage) => stage.semantic === "open");
  const won = pipeline?.stages?.find((stage) => stage.semantic === "won");
  if (!member || !pipeline || !open || !won) {
    return null;
  }
  return {
    member_id: member.id,
    pipeline_id: pipeline.id,
    open_stage_id: open.id,
    won_stage_id: won.id,
  };
}

function useRefresh() {
  const queryClient = useQueryClient();
  return () => queryClient.invalidateQueries({ queryKey: STATUS_KEY });
}

function ConnectCard() {
  const t = useT();
  const refresh = useRefresh();
  const setup = useQuery({ queryKey: SETUP_KEY, queryFn: findSetup });
  const connect = useMutation({
    mutationFn: async (body: Setup) => {
      const { error, response } = await api.PUT("/ext/billing-demo/connection", {
        body,
      });
      if (error || !response.ok) {
        throwProblem(error);
      }
    },
    onSettled: refresh,
  });
  return (
    <Card>
      <SectionHeader title={t("extBillingDemo.connect.title")} />
      <p>{t("extBillingDemo.connect.sub")}</p>
      <QueryStates query={setup} pendingLabel={t("extBillingDemo.connect.title")}>
        {setup.data ? (
          <Row>
            <Button
              disabled={connect.isPending}
              onClick={() => setup.data && connect.mutate(setup.data)}
            >
              {t("extBillingDemo.connect.action")}
            </Button>
          </Row>
        ) : (
          <ErrorLine>{t("extBillingDemo.connect.missing")}</ErrorLine>
        )}
      </QueryStates>
      {connect.isError ? (
        <ErrorLine>{t("extBillingDemo.connect.failed")}</ErrorLine>
      ) : null}
    </Card>
  );
}

function InvoiceActions({
  invoice,
  canWrite,
}: Readonly<{ invoice: Invoice; canWrite: boolean }>) {
  const t = useT();
  const refresh = useRefresh();
  const [draft, setDraft] = useState("");
  const update = useMutation({
    mutationFn: async (change: { paid?: boolean; amount_minor?: number }) => {
      const { error, response } = await api.POST(
        "/ext/billing-demo/invoice-update",
        { body: { erp_id: invoice.erp_id, ...change } },
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
    },
    onSuccess: () => setDraft(""),
    onSettled: refresh,
  });
  if (!canWrite) {
    return null;
  }
  const major = Number(draft.replace(",", "."));
  const valid = draft.trim() !== "" && Number.isFinite(major) && major >= 0;
  return (
    <Stack gap="2">
      <Row>
        {invoice.paid ? null : (
          <Button
            disabled={update.isPending}
            onClick={() => update.mutate({ paid: true })}
          >
            {t("extBillingDemo.invoice.markPaid")}
          </Button>
        )}
        <TextInput
          aria-label={t("extBillingDemo.invoice.amountLabel", {
            id: invoice.erp_id,
          })}
          inputMode="decimal"
          placeholder={t("extBillingDemo.invoice.amountPlaceholder")}
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
        />
        <Button
          variant="ghost"
          disabled={!valid || update.isPending}
          onClick={() => update.mutate({ amount_minor: Math.round(major * 100) })}
        >
          {t("extBillingDemo.invoice.changeAmount")}
        </Button>
      </Row>
      {update.isError ? (
        <ErrorLine>{t("extBillingDemo.invoice.failed")}</ErrorLine>
      ) : null}
    </Stack>
  );
}

function InvoiceList({
  invoices,
  canWrite,
}: Readonly<{ invoices: Invoice[]; canWrite: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <DataTable<Invoice>
      label={t("extBillingDemo.invoices.title")}
      rows={invoices}
      rowKey={(row) => row.erp_id}
      columns={[
        {
          key: "invoice",
          header: t("extBillingDemo.invoices.invoice"),
          render: (row) => `${row.erp_id} · ${row.name}`,
        },
        {
          key: "amount",
          header: t("extBillingDemo.invoices.amount"),
          render: (row) =>
            `${formatNumber(row.amount_minor / 100, locale)} ${row.currency}`,
        },
        {
          key: "paid",
          header: t("extBillingDemo.invoices.paid"),
          render: (row) => (
            <Badge tone={row.paid ? "success" : "warning"}>
              {t(
                row.paid
                  ? "extBillingDemo.invoices.paidYes"
                  : "extBillingDemo.invoices.paidNo",
              )}
            </Badge>
          ),
        },
        {
          key: "deal",
          header: t("extBillingDemo.invoices.deal"),
          render: (row) => (
            <Stack gap="2">
              {row.deal_id ? (
                <a href={`#/deals/${row.deal_id}`}>
                  {t("extBillingDemo.invoices.openDeal")}
                </a>
              ) : null}
              <Badge tone={row.synced ? "success" : "warning"}>
                {t(
                  row.synced
                    ? "extBillingDemo.invoices.synced"
                    : "extBillingDemo.invoices.waiting",
                )}
              </Badge>
              {row.withheld.length > 0 ? (
                <span>{t("extBillingDemo.invoices.kept")}</span>
              ) : null}
            </Stack>
          ),
        },
        {
          key: "actions",
          header: t("extBillingDemo.invoices.actions"),
          render: (row) => <InvoiceActions invoice={row} canWrite={canWrite} />,
        },
      ]}
    />
  );
}

export default function BillingDemoScreen() {
  const t = useT();
  const canWrite = useCanWrite(FEED_OBJECT, "update");
  const status = useQuery({
    refetchInterval: POLL_MS,
    queryKey: STATUS_KEY,
    queryFn: async () => {
      const { data, error, response } = await api.GET("/ext/billing-demo/status");
      if (error || !response.ok) {
        throwProblem(error);
      }
      if (!Array.isArray(data?.invoices)) {
        throw new Error("the status read carried no `invoices` array");
      }
      return data.invoices;
    },
  });
  return (
    <div className="wrap narrow">
      <SectionHeader title={t("extBillingDemo.title")} level={1} />
      <p>{t("extBillingDemo.sub")}</p>
      <QueryStates query={status} pendingLabel={t("extBillingDemo.title")}>
        {status.data && status.data.length > 0 ? (
          <Card>
            <SectionHeader title={t("extBillingDemo.invoices.title")} />
            <p>{t("extBillingDemo.invoices.sub")}</p>
            <InvoiceList invoices={status.data} canWrite={canWrite} />
          </Card>
        ) : (
          canWrite ? <ConnectCard /> : <p>{t("extBillingDemo.noGrant")}</p>
        )}
      </QueryStates>
    </div>
  );
}
