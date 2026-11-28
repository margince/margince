export function channelConnectionsNotConfigured(
  status: number,
  code: string | null | undefined,
): boolean {
  return (
    status === 503 &&
    (code === "channel_connections_not_configured" ||
      code === "channel_credentials_not_configured")
  );
}
