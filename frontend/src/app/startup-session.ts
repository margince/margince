let ready = Promise.resolve();

export function registerStartupSession(preparation: Promise<void>): void {
  ready = preparation;
}

export function startupSessionReady(): Promise<void> {
  return ready;
}
