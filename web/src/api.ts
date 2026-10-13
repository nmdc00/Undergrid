export type ObjectValue = Record<string, unknown>;
export function object(value: unknown): ObjectValue {
  if (typeof value !== "object" || value === null || Array.isArray(value))
    throw new Error("Invalid server response");
  return value as ObjectValue;
}
export function text(value: unknown): string {
  if (typeof value !== "string") throw new Error("Invalid server response");
  return value;
}
export function items(value: ObjectValue): ObjectValue[] {
  if (!Array.isArray(value.items)) throw new Error("Invalid server response");
  return value.items.map(object);
}
export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
  ) {
    super(code);
  }
}
// Cookies carry owner identity. CSRF lives only in memory. No browser bearer key,
// cross-origin target, arbitrary response URL, or credential persistence.
const pendingIntentKeys = new Map<string, string>();
export async function request(
  path: string,
  csrf = "",
  method = "GET",
  body?: unknown,
  match?: string,
): Promise<ObjectValue> {
  if (!path.startsWith("/v1/") && !path.startsWith("/auth/"))
    throw new Error("Invalid API target");
  const headers = new Headers({ Accept: "application/json" });
  const retryIdentity =
    (path.startsWith("/v1/owner/intents") ||
      path.startsWith("/v1/owner/agents/") ||
      path.endsWith("/rotate")) &&
    method !== "GET"
      ? JSON.stringify([csrf, method, path, body, match])
      : "";
  const commandKey = retryIdentity
    ? (pendingIntentKeys.get(retryIdentity) ?? crypto.randomUUID())
    : crypto.randomUUID();
  if (retryIdentity) {
    if (pendingIntentKeys.size >= 64 && !pendingIntentKeys.has(retryIdentity))
      throw new Error(
        "Too many uncertain commands; refresh and review server state",
      );
    pendingIntentKeys.set(retryIdentity, commandKey);
  }
  if (method !== "GET") {
    headers.set("X-CSRF-Token", csrf);
    headers.set("Content-Type", "application/json");
    headers.set("Idempotency-Key", commandKey);
  }
  if (match) headers.set("If-Match", match);
  const response = await fetch(path, {
    method,
    headers,
    credentials: "same-origin",
    redirect: "error",
    cache: "no-store",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (retryIdentity && response.status < 500)
    pendingIntentKeys.delete(retryIdentity);
  if (response.status === 204) return {};
  const value: unknown = await response.json().catch(() => ({}));
  const result = object(value);
  if (!response.ok) {
    const code =
      typeof result.error === "object" && result.error !== null
        ? object(result.error).code
        : undefined;
    throw new ApiError(
      response.status,
      typeof code === "string" ? code : `HTTP ${response.status}`,
    );
  }
  return result;
}
