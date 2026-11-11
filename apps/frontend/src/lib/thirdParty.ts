import type { PublicThirdPartySource } from "../api/types";

/** Events in progress at providers with fresh data, for "Acontecendo agora". */
export function openThirdPartyEvents(sources: PublicThirdPartySource[]) {
  return sources
    .filter((s) => !s.stale)
    .flatMap((s) => s.events.map((event, i) => ({ key: `${s.id}-${i}`, source: s, event })));
}

