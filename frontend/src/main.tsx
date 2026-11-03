import { QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { api } from "./api/client";
import { AttentionProvider } from "./app/attention";
import { AppErrorBoundary } from "./app/errorboundary";
import { createQueryClient } from "./app/queryclient";
import { BulkUndoProvider } from "./components/bulkchange";
import { FilePreviewProvider } from "./design-system/filepreview";
import { ToastProvider, ToastRegion } from "./design-system/toast";
import { LocaleProvider } from "./i18n";
import { listenForInstall, registerServiceWorker } from "./lib/pwa";
import { restoreReturnRoute } from "./lib/returnroute";
import { sessionRefusalWatch } from "./lib/session";
import { reloadOnStaleBuild } from "./lib/stalebuild";
import { startTheme } from "./lib/theme";
import "./app.css";

const queryClient = createQueryClient();

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
reloadOnStaleBuild();

api.use(sessionRefusalWatch(queryClient));

const root = document.getElementById("root");
if (!root) {
  throw new Error("index.html must provide #root");
}
createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      {/* Inside the query provider so it can follow the signed-in person's own
          locale from `/me` — the choice has to survive moving browsers, and
          localStorage alone cannot do that. */}
      <LocaleProvider>
        {/* Inside the locale provider: the fallback is translated copy like
            every other user-facing string, so it cannot render above the
            catalog it reads from. */}
        <AppErrorBoundary>
          {/* Inside the boundary, outside App: the chrome that reads attention
              and the screens that publish it are both under App, and a provider
              that mounted with them would remount every time App did. */}
          <AttentionProvider>
            {/* Outside App for the same reason as the provider above it, and
                the region is a SIBLING of App rather than a child: it is
                portalled to the body either way, but rendering it here is what
                makes "exactly one region exists" a fact about this file rather
                than a habit each screen has to keep. */}
            <ToastProvider>
              {/* Around App rather than beside it, because what it gives the
                  tree is a context every file card reads: a chip under a
                  provider that was not an ancestor is a chip that quietly
                  stays a download. It draws its own dialog, portalled like
                  every other overlay here. */}
              <FilePreviewProvider>
                {/* The undo a bulk change offers opens here, not in the list
                    that ran it: that list's bulk bar is gone once the change
                    clears its selection. */}
                <BulkUndoProvider>
                  <App />
                </BulkUndoProvider>
              </FilePreviewProvider>
              <ToastRegion />
            </ToastProvider>
          </AttentionProvider>
        </AppErrorBoundary>
      </LocaleProvider>
    </QueryClientProvider>
  </StrictMode>,
);
