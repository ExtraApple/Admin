import type { RouteState } from "./models";

export interface ApiListQueryState {
  page: number;
  size: number;
  keyword: string;
  group: string;
  method: string;
  status: "" | "0" | "1";
  needAuth: "" | "0" | "1";
  needAudit: "" | "0" | "1";
  routeState: "" | RouteState;
}

export const DEFAULT_API_LIST_QUERY: ApiListQueryState = {
  page: 1,
  size: 20,
  keyword: "",
  group: "",
  method: "",
  status: "",
  needAuth: "",
  needAudit: "",
  routeState: "",
};

const routeStates: readonly RouteState[] = [
  "matched",
  "metadata_only",
  "catalog_only",
  "access_mismatch",
];

function queryParams(search: string | URLSearchParams): URLSearchParams {
  return typeof search === "string"
    ? new URLSearchParams(search.startsWith("?") ? search.slice(1) : search)
    : new URLSearchParams(search);
}

function positiveInt(value: string | null, fallback: number, max: number): number {
  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed < 1) return fallback;
  return Math.min(parsed, max);
}

function binaryFilter(value: string | null): "" | "0" | "1" {
  return value === "0" || value === "1" ? value : "";
}

function routeStateFilter(value: string | null): "" | RouteState {
  return value && routeStates.includes(value as RouteState) ? (value as RouteState) : "";
}

export function parseApiListQuery(search: string | URLSearchParams): ApiListQueryState {
  const params = queryParams(search);
  return {
    page: positiveInt(params.get("page"), DEFAULT_API_LIST_QUERY.page, 1000000),
    size: positiveInt(params.get("size"), DEFAULT_API_LIST_QUERY.size, 50),
    keyword: params.get("keyword") ?? "",
    group: params.get("group") ?? "",
    method: (params.get("method") ?? "").toUpperCase(),
    status: binaryFilter(params.get("status")),
    needAuth: binaryFilter(params.get("need_auth")),
    needAudit: binaryFilter(params.get("need_audit")),
    routeState: routeStateFilter(params.get("route_state")),
  };
}

export function encodeApiListQuery(state: Partial<ApiListQueryState>): string {
  const query = { ...DEFAULT_API_LIST_QUERY, ...state };
  const params = new URLSearchParams();
  params.set("page", String(query.page));
  params.set("size", String(query.size));
  if (query.keyword) params.set("keyword", query.keyword);
  if (query.group) params.set("group", query.group);
  if (query.method) params.set("method", query.method.toUpperCase());
  if (query.status) params.set("status", query.status);
  if (query.needAuth) params.set("need_auth", query.needAuth);
  if (query.needAudit) params.set("need_audit", query.needAudit);
  if (query.routeState) params.set("route_state", query.routeState);
  return params.toString();
}

export function parseMenuSelectionQuery(search: string | URLSearchParams): number | null {
  const value = Number(queryParams(search).get("menu_id"));
  return Number.isInteger(value) && value > 0 ? value : null;
}

export function updateMenuSelectionQuery(search: string | URLSearchParams, menuId: number | null): string {
  const params = queryParams(search);
  if (menuId && Number.isInteger(menuId) && menuId > 0) {
    params.set("menu_id", String(menuId));
  } else {
    params.delete("menu_id");
  }
  return params.toString();
}
