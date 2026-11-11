import { useMemo, useState } from "react";
import { publicApi, type UptimeRange } from "../api/endpoints";
import { useApi } from "../lib/useApi";
import { viewerTimeZone } from "../lib/uptime";
import { RANGES, type Bars } from "./uptime";

const RANGE_KEY = "dsp.uptime.days";

function savedRange(): UptimeRange {
  try {
    const v = Number(localStorage.getItem(RANGE_KEY));
    return (RANGES as number[]).includes(v) ? (v as UptimeRange) : 90;
  } catch {
    return 90;
  }
}

/**
 * Loads the daily bars of public components (in the viewer's timezone) for
 * the chosen range and offers the 30/60/90-day picker. The bars complement
 * the page: when they cannot be loaded, components are shown without them.
 */
export function useUptime() {
  const [days, setDaysState] = useState<UptimeRange>(savedRange);
  const tz = useMemo(() => viewerTimeZone(), []);
  const { state } = useApi((signal) => publicApi.uptime(days, tz, { signal }), [days, tz]);
  const bars = useMemo<Bars | null>(
    () => (state.status === "success" ? new Map(state.data.components.map((c) => [c.id, c])) : null),
    [state],
  );
  const setDays = (d: UptimeRange) => {
    setDaysState(d);
    try {
      localStorage.setItem(RANGE_KEY, String(d));
    } catch {
      /* a convenience only */
    }
  };
  return { days, setDays, bars, failed: state.status === "error" };
}

