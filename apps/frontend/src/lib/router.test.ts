import { describe, expect, it } from "vitest";
import { match } from "./router";

describe("match", () => {
  it("extracts parameters", () => {
    expect(match("/admin/incidents/:id", "/admin/incidents/abc")).toEqual({ id: "abc" });
    expect(match("/admin/incidents/:id", "/admin/incidents/abc/")).toEqual({ id: "abc" });
  });

  it("requires every segment to match", () => {
    expect(match("/admin/incidents/:id", "/admin/incidents")).toBeNull();
    expect(match("/admin/incidents", "/admin/maintenances")).toBeNull();
    expect(match("/admin", "/admin")).toEqual({});
  });
});
