import { createContext, useContext, useMemo, type ReactNode } from "react";
import type { UptimeRange } from "../api/endpoints";
import type { Uptime } from "../api/types";
import { UptimeBar } from "../components/UptimeBar";
import { aggregate } from "../lib/uptime";

export const RANGES: UptimeRange[] = [30, 60, 90];

export type Bars = Map<string, Uptime["components"][number]>;
const UptimeContext = createContext<Bars | null>(null);

export function UptimeProvider({ bars, children }: { bars: Bars | null; children: ReactNode }) {
  return <UptimeContext.Provider value={bars}>{children}</UptimeContext.Provider>;
}

export function RangePicker({ days, onChange }: { days: UptimeRange; onChange: (d: UptimeRange) => void }) {
  return (
    <div className="range-picker" role="group" aria-label="Período do histórico">
      {RANGES.map((r) => (
        <button key={r} type="button" aria-pressed={r === days} onClick={() => onChange(r)}>
          {r} dias
        </button>
      ))}
    </div>
  );
}

/** A component's own bar, with the day details. */
export function ComponentUptime({ id, name }: { id: string; name: string }) {
  const bar = useContext(UptimeContext)?.get(id);
  if (!bar) return null;
  return <UptimeBar name={name} days={bar.days} uptimePercent={bar.uptime_percent} details incidentLink={(i) => `/incidents/${i}`} />;
}

/** The consolidated bar of a group or subgroup (see lib/uptime aggregate). */
export function GroupUptime({ ids, name, size = "md" }: { ids: string[]; name: string; size?: "md" | "sm" }) {
  const all = useContext(UptimeContext);
  const agg = useMemo(() => {
    if (!all) return null;
    return aggregate(ids.map((id) => all.get(id)).filter((b): b is NonNullable<typeof b> => b !== undefined));
  }, [all, ids]);
  if (!agg) return null;
  return <UptimeBar name={name} days={agg.days} uptimePercent={agg.uptimePercent} size={size} details incidentLink={(i) => `/incidents/${i}`} />;
}
