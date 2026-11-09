import { extensionInstruments } from "@composition/instruments";
import { QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { AttentionProvider } from "./app/attention";
import { AppErrorBoundary } from "./app/errorboundary";
import { queryClient } from "./app/queryclient.instance";
import { startupSessionReady } from "./app/startup-session";
import { BulkUndoProvider } from "./components/bulkundo";
import { FilePreviewProvider } from "./design-system/filepreview";
import { ToastProvider, ToastRegion } from "./design-system/toast";
import { LocaleProvider } from "./i18n";
import { localeReady } from "./i18n/catalogs";
import { preferredLocale } from "./i18n/locale";
import {
  type BrowserInstrument,
  reportError,
  startInstruments,
} from "./lib/instruments";
import { listenForInstall, registerServiceWorker } from "./lib/pwa";
import { SPA_RELEASE } from "./lib/release";
import { restoreReturnRoute } from "./lib/returnroute";
import { parseHash } from "./lib/router";
import { startTheme } from "./lib/theme";
import "./app.css";

// BEFORE the first render, and before any authentication is known. The theme
// used to be applied by an effect inside the signed-in chrome, so every
// unauthenticated surface — sign-in, password reset, the availability screens —
// ignored a dark-mode reader entirely, and inherited a stale attribute after a
// sign-out. Setting it here also avoids a light-to-dark flash on reload for a
// reader who has chosen dark.
//
// It also starts the `prefers-color-scheme` subscription that a reader whose
// choice is "system" is asking for. That belongs at boot rather than to the
// first mounted theme control, because the account menu renders its control
// only while it is open — a tab nobody opened the menu in would otherwise stay
// deaf to the switch.
startTheme();

// Before the first render reads the address: a provider sign-in lands bare.
restoreReturnRoute();

listenForInstall();
registerServiceWorker();

// Extension instruments start before the first render, so an error in it is
// theirs to see. The generated registry is untyped; it is annotated here.
const composedInstruments: Readonly<Record<string, BrowserInstrument>> =
  extensionInstruments;
startInstruments(composedInstruments, SPA_RELEASE);

const root = document.getElementById("root");
if (!root) {
  throw new Error("index.html must provide #root");
}
async function mountApp(root: HTMLElement) {
  try {
    await Promise.all([
      localeReady(
        preferredLocale(),
        parseHash(location.hash).screen === "analytics"
          ? "essential"
          : "complete",
      ),
      startupSessionReady(),
    ]);
  } catch (error: unknown) {
    console.error(
      "Startup language could not be loaded; the error boundary offers Retry",
      error,
    );
  }
  createRoot(root, {
    // These keep React's production behaviour (a caught error goes to
    // console.error, an uncaught one to reportError) and also tell the
    // instruments. A boundary-caught error is still the boundary's to draw.
    onCaughtError: (error, info) => {
      console.error(error);
      reportError(error, {
        componentStack: info.componentStack ?? undefined,
        caught: true,
      });
    },
    onUncaughtError: (error, info) => {
      reportError(error, {
        componentStack: info.componentStack ?? undefined,
        caught: false,
      });
      globalThis.reportError(error);
    },
  }).render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <AppErrorBoundary>
          <LocaleProvider
            initialEssential={parseHash(location.hash).screen === "analytics"}
          >
            <AppErrorBoundary>
              <AttentionProvider>
                <ToastProvider>
                  <FilePreviewProvider>
                    <BulkUndoProvider>
                      <App />
                    </BulkUndoProvider>
                  </FilePreviewProvider>
                  <ToastRegion />
                </ToastProvider>
              </AttentionProvider>
            </AppErrorBoundary>
          </LocaleProvider>
        </AppErrorBoundary>
      </QueryClientProvider>
    </StrictMode>,
  );
}
void mountApp(root);
