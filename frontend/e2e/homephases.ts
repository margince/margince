import type { CDPSession, Page, Request } from "@playwright/test";
import type { JourneyMark } from "../src/lib/journeymarks";
import type { RetainedAnswer } from "../src/testing/homeanswers.judge";
import {
  HOME_READS,
  type HomeRead,
  type Journey,
  readOf,
} from "../src/testing/homejourney.judge";
import {
  type BrowserRequestTiming,
  elapsedPhase,
  type RequestMeasurement,
  readinessPhases,
  requestPhases,
} from "../src/testing/homephases.judge";
import type { HomeRequestObserver } from "./homejourney";

const FETCH_MARK = "margince:bench-fetch";
const FETCH_FAILURE = "margince:bench-fetch-failed";
type NetworkSend = {
  id: string;
  url: string;
  method: string;
  loader: string;
  sendMs: number | null;
  cached: boolean | null;
  serviceWorker: boolean | null;
  correlation: string | null;
};
type ObservedRequest = {
  read: HomeRead;
  journey: string;
  request: Request;
  attempt: number;
  status: number | null;
  finished: boolean;
};
export type PhaseJourney = {
  marks: Partial<Record<JourneyMark, number>>;
  anchorMs: number;
  kind: "load" | "return" | "refresh";
  outcome: Journey["outcome"];
  reasons: readonly string[];
  times?: Journey["times"];
  retainedAnswers?: readonly RetainedAnswer[];
  cacheAgeAdvancedMs?: number;
};
const unavailableTiming: BrowserRequestTiming = {
  startTime: -1,
  requestStart: -1,
  responseStart: -1,
  responseEnd: -1,
};
const declared = (value: string | undefined) => value?.trim() || null;

export class HomePhaseLog implements HomeRequestObserver {
  private readonly requests = new Map<Request, ObservedRequest>();
  private readonly network: NetworkSend[] = [];
  private readonly activeNetwork = new Map<string, NetworkSend>();
  private readonly intercepted = new WeakSet<Request>();
  private readonly documents = new Map<number, number>();

  private constructor(
    private readonly page: Page,
    private readonly session: CDPSession,
  ) {}

  static async install(page: Page): Promise<HomePhaseLog> {
    const session = await page.context().newCDPSession(page);
    const log = new HomePhaseLog(page, session);
    session.on("Network.requestWillBeSent", (event) => {
      if (
        !readOf(new URL(event.request.url), event.request.method.toUpperCase())
      )
        return;
      const sent: NetworkSend = {
        id: event.requestId,
        url: event.request.url,
        method: event.request.method.toUpperCase(),
        loader: event.loaderId,
        sendMs: null,
        cached: null,
        serviceWorker: null,
        correlation: null,
      };
      log.network.push(sent);
      log.activeNetwork.set(event.requestId, sent);
    });
    session.on("Network.responseReceived", (event) => {
      const sent = log.activeNetwork.get(event.requestId);
      if (!sent) return;
      const timing = event.response.timing;
      sent.sendMs =
        timing && timing.sendStart >= 0 && timing.sendEnd >= timing.sendStart
          ? timing.sendEnd - timing.sendStart
          : null;
      sent.cached =
        sent.cached === true || event.response.fromDiskCache === true;
      sent.serviceWorker = event.response.fromServiceWorker ?? false;
      const correlation = Object.entries(event.response.headers).find(
        ([name]) => name.toLowerCase() === "x-correlation-id",
      )?.[1];
      sent.correlation =
        typeof correlation === "string" &&
        /^[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}$/i.test(correlation)
          ? correlation
          : null;
    });
    session.on("Network.requestServedFromCache", (event) => {
      const sent = log.activeNetwork.get(event.requestId);
      if (sent) sent.cached = true;
    });
    await session.send("Network.enable");
    await page.addInitScript(
      ({ mark, failure }) => {
        const nativeFetch = window.fetch.bind(window);
        window.fetch = (input, init) => {
          const url = new URL(
            input instanceof Request ? input.url : String(input),
            location.href,
          ).href;
          performance.mark(mark, {
            detail: {
              url,
              method: (
                init?.method ??
                (input instanceof Request ? input.method : "GET")
              ).toUpperCase(),
              at: performance.now(),
            },
          });
          return nativeFetch(input, init).catch((error: unknown) => {
            performance.mark(failure, {
              detail:
                error instanceof DOMException && error.name === "AbortError"
                  ? "cancelled"
                  : "failed",
            });
            throw error;
          });
        };
      },
      { mark: FETCH_MARK, failure: FETCH_FAILURE },
    );
    return log;
  }

  started(read: HomeRead, journey: string, request: Request, attempt: number) {
    this.requests.set(request, {
      read,
      journey,
      request,
      attempt,
      status: null,
      finished: false,
    });
  }
  responded(request: Request, status: number) {
    const observed = this.requests.get(request);
    if (observed) observed.status = status;
  }
  finished(request: Request) {
    const observed = this.requests.get(request);
    if (observed) observed.finished = true;
  }
  readonly intercept = (request: Request) => {
    this.intercepted.add(request);
  };

  private associated() {
    const all = [...this.requests.values()];
    return new Map(
      all.map((observed) => {
        const peers = all.filter(
          (item) =>
            item.request.url() === observed.request.url() &&
            item.request.method() === observed.request.method(),
        );
        const network = this.network.filter(
          (item) =>
            item.url === observed.request.url() &&
            item.method === observed.request.method(),
        );
        return [
          observed.request,
          peers.length === network.length
            ? network[peers.indexOf(observed)]
            : undefined,
        ];
      }),
    );
  }

  async snapshot(journey: string, state: PhaseJourney) {
    const { frameTree } = await this.session.send("Page.getFrameTree");
    const browser = await this.page.evaluate(
      ({ mark, failure }) => ({
        origin: performance.timeOrigin,
        fetchCancelled: performance
          .getEntriesByName(failure, "mark")
          .filter(
            (entry) =>
              entry instanceof PerformanceMark && entry.detail === "cancelled",
          ).length,
        calls: performance.getEntriesByName(mark, "mark").flatMap((entry) => {
          if (!(entry instanceof PerformanceMark)) return [];
          const detail: unknown = entry.detail;
          if (
            !detail ||
            typeof detail !== "object" ||
            !("url" in detail) ||
            typeof detail.url !== "string" ||
            !("method" in detail) ||
            typeof detail.method !== "string" ||
            !("at" in detail) ||
            typeof detail.at !== "number"
          )
            return [];
          return [{ url: detail.url, method: detail.method, at: detail.at }];
        }),
        resources: performance.getEntriesByType("resource").flatMap((entry) =>
          entry instanceof PerformanceResourceTiming
            ? [
                {
                  url: entry.name,
                  initiator: entry.initiatorType,
                  startTime: entry.startTime,
                  fetchStart: entry.fetchStart,
                  requestStart:
                    entry.requestStart > 0 ? entry.requestStart : -1,
                  domainLookupStart:
                    entry.domainLookupStart > 0 ? entry.domainLookupStart : -1,
                  domainLookupEnd:
                    entry.domainLookupEnd > 0 ? entry.domainLookupEnd : -1,
                  connectStart:
                    entry.connectStart > 0 ? entry.connectStart : -1,
                  connectEnd: entry.connectEnd > 0 ? entry.connectEnd : -1,
                  secureConnectionStart:
                    entry.secureConnectionStart > 0
                      ? entry.secureConnectionStart
                      : -1,
                  responseStart:
                    entry.responseStart > 0 ? entry.responseStart : -1,
                  responseEnd: entry.responseEnd > 0 ? entry.responseEnd : -1,
                  transferBytes: entry.transferSize,
                  serverTiming: entry.serverTiming.map((timing) => ({
                    name: timing.name,
                    durationMs: timing.duration,
                  })),
                },
              ]
            : [],
        ),
        navigation: performance
          .getEntriesByType("navigation")
          .flatMap((entry) =>
            entry instanceof PerformanceNavigationTiming ? [entry.type] : [],
          ),
      }),
      { mark: FETCH_MARK, failure: FETCH_FAILURE },
    );
    if (!this.documents.has(browser.origin))
      this.documents.set(browser.origin, this.documents.size);
    const associated = this.associated();
    const all = [...this.requests.values()];
    const documentRequests = all.filter(
      (item) =>
        associated.get(item.request)?.loader === frameTree.frame.loaderId,
    );
    const responseEnds: number[] = [];
    const measurements = all
      .filter((item) => item.journey === journey)
      .map((observed) => {
        const { read, request, status, attempt, finished } = observed;
        const sent = associated.get(request);
        const peers = documentRequests.filter(
          (item) => item.request.url() === request.url(),
        );
        const resources = browser.resources
          .filter(
            (item) => item.initiator === "fetch" && item.url === request.url(),
          )
          .sort((left, right) => left.startTime - right.startTime);
        const position = peers.indexOf(observed);
        const resource =
          sent?.loader === frameTree.frame.loaderId &&
          position >= 0 &&
          resources.length === peers.length
            ? resources[position]
            : undefined;
        const intercepted = this.intercepted.has(request);
        const measured: RequestMeasurement = {
          read,
          journey,
          status,
          failed: request.failure() !== null,
          anchorMs: state.anchorMs,
          intercepted,
          timing: resource ?? unavailableTiming,
          sendEnd:
            resource &&
            resource.requestStart >= 0 &&
            sent?.sendMs !== null &&
            sent?.sendMs !== undefined
              ? resource.requestStart + sent.sendMs
              : null,
        };
        if (read !== "me") responseEnds.push(resource?.responseEnd ?? -1);
        return {
          read,
          browserRequestId: sent?.id ?? null,
          status,
          attempt,
          inFlight: !finished,
          abandonedDocument:
            sent !== undefined && sent.loader !== frameTree.frame.loaderId,
          failed: measured.failed,
          cancelled: request.failure()?.errorText === "net::ERR_ABORTED",
          intercepted,
          startSinceDocumentMs: resource?.startTime ?? null,
          cached: sent?.cached ?? null,
          serviceWorker: sent?.serviceWorker ?? null,
          correlation: sent?.correlation ?? null,
          serverTiming: intercepted ? null : (resource?.serverTiming ?? null),
          transferBytes: resource?.transferBytes ?? null,
          harnessElapsedMs:
            intercepted && resource
              ? resource.responseEnd - resource.startTime
              : null,
          phases: requestPhases(measured),
        };
      });
    const inFlight = measurements.filter((item) => item.inFlight).length;
    const fetchCalls = browser.calls.filter((call) =>
      readOf(new URL(call.url), call.method),
    ).length;
    const unmatched = fetchCalls - documentRequests.length;
    return {
      journey,
      outcome: state.outcome,
      reasons: state.reasons,
      journeyTimings: state.times ?? null,
      retainedAnswers: state.retainedAnswers ?? [],
      incomplete: inFlight > 0,
      judgedComplete: state.outcome === "complete" && inFlight === 0,
      marksSinceDocumentMs: state.marks,
      anchorSinceDocumentMs: state.anchorMs,
      metadata: {
        declaredCorpus: declared(process.env.BENCH_HOME_CORPUS),
        declaredBuild: declared(process.env.GITHUB_SHA),
        bundles: [
          ...new Set(
            browser.resources
              .map((item) => new URL(item.url).pathname)
              .filter((path) => path.startsWith("/assets/")),
          ),
        ].sort(),
        navigation: browser.navigation,
        documentSequence: this.documents.get(browser.origin),
        initialDocument: this.documents.get(browser.origin) === 0,
        browserCacheClearing: "not-cleared-by-recorder",
        applicationCache: state.kind === "load" ? "new-document" : "retained",
        cacheAgeAdvancedMs: state.cacheAgeAdvancedMs ?? 0,
      },
      census: {
        requests: measurements.length,
        inFlight,
        abandonedDocuments: measurements.filter(
          (item) => item.abandonedDocument,
        ).length,
        retries: measurements.filter((item) => item.attempt > 1).length,
        failed: measurements.filter((item) => item.failed).length,
        serverErrors: measurements.filter(
          (item) => item.status !== null && item.status >= 500,
        ).length,
        cancelled: measurements.filter((item) => item.cancelled).length,
        fetchCallsInDocument: fetchCalls,
        fetchCancelledInDocument: browser.fetchCancelled,
        unmatchedFetchCallsInDocument: unmatched >= 0 ? unmatched : null,
      },
      requests: measurements,
      readiness: {
        ...readinessPhases(
          responseEnds,
          state.marks["home:useful"] ?? null,
          state.marks["home:complete"] ?? null,
          state.outcome === "failed" || state.outcome === "timed out",
        ),
        usefulSinceAnchorMs:
          state.marks["home:useful"] === undefined
            ? null
            : elapsedPhase(state.anchorMs, state.marks["home:useful"]),
        completeSinceAnchorMs:
          state.marks["home:complete"] === undefined
            ? null
            : elapsedPhase(state.anchorMs, state.marks["home:complete"]),
        essentialReads: HOME_READS,
      },
    };
  }

  async close() {
    await this.session.detach();
  }
}
