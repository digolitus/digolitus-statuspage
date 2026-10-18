import { describe, expect, it } from "vitest";
import { componentStatusLabel, formatDateTime, overallLabel, toZonedInput } from "./format";

describe("formatDateTime", () => {
  it("shows the instant in the requested IANA zone with the zone name", () => {
    expect(formatDateTime("2030-01-10T05:00:00Z", "America/Sao_Paulo")).toContain("10/01/2030");
    expect(formatDateTime("2030-01-10T05:00:00Z", "America/Sao_Paulo")).toContain("02:00");
    expect(formatDateTime("2030-01-10T05:00:00Z", "UTC")).toContain("05:00");
  });

  it("handles missing and invalid values", () => {
    expect(formatDateTime(null)).toBe("—");
    expect(formatDateTime("not a date")).toBe("—");
  });
});

describe("toZonedInput", () => {
  it("converts an instant to wall time in the given zone, independent of the browser", () => {
    expect(toZonedInput("2030-01-10T05:00:00Z", "America/Sao_Paulo")).toBe("2030-01-10T02:00");
    expect(toZonedInput("2026-07-15T13:00:00Z", "America/New_York")).toBe("2026-07-15T09:00");
    expect(toZonedInput("2026-01-15T14:00:00Z", "America/New_York")).toBe("2026-01-15T09:00");
  });
});

describe("labels", () => {
  it("never claims verified availability", () => {
    expect(overallLabel.operational).toContain("declarados");
    expect(overallLabel.unknown).toBe("Status não informado");
    expect(componentStatusLabel.maintenance).toBe("Em manutenção");
  });
});
