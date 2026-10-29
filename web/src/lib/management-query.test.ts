import { describe, expect, it } from "vitest";
import {
  encodeApiListQuery,
  parseApiListQuery,
  parseMenuSelectionQuery,
  updateMenuSelectionQuery,
} from "./management-query";

describe("management URL query state", () => {
  it("round-trips API filters, pagination, and encoded text", () => {
    const query = encodeApiListQuery({
      page: 3,
      size: 50,
      keyword: "用户 管理",
      group: "system/admin",
      method: "get",
      status: "1",
      needAuth: "0",
      needAudit: "1",
      routeState: "access_mismatch",
    });

    expect(parseApiListQuery(query)).toEqual({
      page: 3,
      size: 50,
      keyword: "用户 管理",
      group: "system/admin",
      method: "GET",
      status: "1",
      needAuth: "0",
      needAudit: "1",
      routeState: "access_mismatch",
    });
  });

  it("rejects invalid filter values and preserves safe defaults", () => {
    expect(parseApiListQuery("?page=0&size=500&method=wat&status=2&route_state=unknown")).toEqual({
      page: 1,
      size: 50,
      keyword: "",
      group: "",
      method: "WAT",
      status: "",
      needAuth: "",
      needAudit: "",
      routeState: "",
    });
  });

  it("restores and updates menu selection without dropping other query state", () => {
    expect(parseMenuSelectionQuery("?keyword=users&menu_id=42")).toBe(42);
    expect(updateMenuSelectionQuery("?keyword=users", 42)).toBe("keyword=users&menu_id=42");
    expect(updateMenuSelectionQuery("?keyword=users&menu_id=42", null)).toBe("keyword=users");
    expect(parseMenuSelectionQuery("?menu_id=invalid")).toBeNull();
  });
});
