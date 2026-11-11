import { describe, expect, it } from "vitest";
import type { UptimeDay } from "../api/types";
import { aggregate, formatPercent, formatSeconds } from "./uptime";

const zero = { major_outage: 0, partial_outage: 0, degraded_performance: 0, maintenance: 0 };
const day = (over: Partial<UptimeDay>): UptimeDay => ({ date: "2026-10-30", status: "operational", uptime_percent: 100, durations: zero, incidents: [], ...over });

describe("aggregate", () => {
  it("takes the worst status of the day, sums durations, merges incidents and averages uptime", () => {
    const inc = { id: "i1", title: "Falha", impact: "partial_outage" as const };
    const agg = aggregate([
      { days: [day({ status: "partial_outage", uptime_percent: 91, durations: { ...zero, partial_outage: 600 }, incidents: [inc] })], uptime_percent: 91 },
      { days: [day({ status: "maintenance", durations: { ...zero, maintenance: 300 }, incidents: [inc] })], uptime_percent: 100 },
      { days: [day({ status: "no_data", uptime_percent: null })], uptime_percent: null },
    ]);
    expect(agg?.days[0].status).toBe("partial_outage");
    expect(agg?.days[0].durations).toEqual({ ...zero, partial_outage: 600, maintenance: 300 });
    expect(agg?.days[0].incidents).toEqual([inc]);
    expect(agg?.days[0].uptime_percent).toBe(95.5);
    expect(agg?.uptimePercent).toBe(95.5);
  });

  it("returns null without components and no data when none has data", () => {
    expect(aggregate([])).toBeNull();
    expect(aggregate([{ days: [day({ status: "no_data", uptime_percent: null })], uptime_percent: null }])?.uptimePercent).toBeNull();
  });
});

describe("formatting", () => {
  it("never rounds downtime up to 100% and formats durations", () => {
    expect(formatPercent(99.99)).toBe("99,99%");
    expect(formatPercent(null)).toBe("sem dados");
    expect(formatSeconds(20)).toBe("menos de 1 min");
    expect(formatSeconds(2700)).toBe("45 min");
    expect(formatSeconds(5400)).toBe("1 h 30 min");
  });
});
