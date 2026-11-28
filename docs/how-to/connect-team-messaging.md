<!-- describes: frontend/src/features/settings/channelinstallations.tsx frontend/src/features/settings/messagingserviceswitch.tsx backend/internal/compose/slackchanneloauth.go backend/internal/compose/teamsapp/package.go backend/internal/modules/chatlinks/slack/manifest.go -->
# Connect team messaging

Use this page to prepare an app registration for Messenger and channel sync. For everyday use and limits, read [Messaging apps](../handbook/messaging-apps.md).

## Choose the service

Open **Settings**, **Integrations**, **Messaging accounts** as an administrator. Choose the workspace's team messaging service and confirm. Switching disconnects the old service's shared and personal accounts and ends its channel links. History remains. Records can be linked to the new service. If cleanup fails, resolve the reported problem and retry the same selection.

On upgrade, the earliest shared connection keeps the workspace service choice. When there is no shared connection, the earliest retained personal consent determines it. Other services remain blocked until an administrator confirms a choice and completes cleanup.

A shared connection runs channel sync. Each colleague connects an account to send personal messages. Configure both paths with the same app and Microsoft directory.

## Slack

1. Create a Slack app for the intended workspace. Save its app credentials and signing secret in Margince's Slack app settings.
2. Download **Slack app setup** from the shared channel setup. Apply the generated manifest to the app. It declares both OAuth redirect addresses, the bot and personal permissions, and the event subscription address. Keep the generated addresses unchanged.
3. Enable the event subscriptions. Margince can answer Slack's signed verification challenge before the bot is installed, using the configured signing secret.
4. Install or reconnect the shared Slack bot from Margince. Declare the app's actual distribution when asked. An app created from a manifest is not necessarily an internal app: a commercially distributed app without Marketplace approval can have lower history limits.
5. Add the bot to each channel to be linked. Each colleague connects a personal Slack account from **Messaging accounts**.
6. Link a channel from a company, deal or project. Public channels support incoming summaries; outgoing CRM sharing requires a private internal audience and its separate review.

Record the actual distribution under the service selector before using personal Messenger, including when no shared bot is installed. Without confirmation, personal reads use conservative limits. This setting does not change Slack's limits.

Existing private-channel bot installations can refresh with their existing permissions. Reinstall to add public-channel reading. Old installations may retain a delegated history-reader consent in Slack; Margince no longer uses it. Reconnect the shared bot to replace older locally saved credentials. Review and revoke an obsolete grant in Slack after checking whether it also authorizes personal Messenger.

Reinstall after changing app permissions. A successful send does not prove reply-reading access. Check a thread with replies and a channel with enough messages to need another page.

## Microsoft Teams

1. Register a Microsoft app in the intended Microsoft directory. Configure its directory ID and app credentials in Margince's Microsoft app settings.
2. Have a directory administrator grant the delegated permissions displayed for personal Messenger. Colleagues cannot complete consent when the Microsoft directory requires administrator approval and it is missing.
3. Register the app's Teams bot with the event address shown in Margince. Download the Teams app package, upload it to Teams and add it to each intended team. Review the package's resource-specific permissions when installing.
4. Refresh installed teams in Margince and approve each intended team. The connection can include several approved teams in the same Microsoft directory.
5. Add the app separately to each private or shared channel you intend to link. Installation in the parent team does not grant access to those channels.
6. Connect a personal Teams account, then link an existing channel. Create channels and manage their members in Teams. Review the complete audience before enabling automatic CRM posts.

Teams message notifications can be unavailable even when channel reads work. Margince continues polling in that case. An outside or unknown audience blocks automatic sharing; channel access alone does not grant permission to disclose a CRM record.

The manifest and membership requirements follow [Microsoft's channel app guidance](https://learn.microsoft.com/en-us/microsoftteams/platform/build-apps-for-shared-private-channels). Slack's distribution limits are described in its [history rate-limit notice](https://docs.slack.dev/changelog/2025/05/29/rate-limit-changes-for-non-marketplace-apps/).

## Check the connection

Use test conversations before enabling automatic sharing. Confirm the sender, a direct message, a group, replies, member changes, a file-only message, reconnect and revoked access. Check that a denied channel leaves other channels working, and that switching service rejects an old consent callback.

Local HTTP fixtures cover provider responses without accounts. They do not replace these checks against the registered apps. Keep automatic sharing off until the intended audience and access have been checked.
