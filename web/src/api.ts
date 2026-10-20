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
const pendingOwnerAgentKeys = new Map<
  string,
  { identity: string; key: string }
>();
export async function request(
  path: string,
  csrf = "",
  method = "GET",
  body?: unknown,
  match?: string,
  validateResult?: (result: ObjectValue) => void,
): Promise<ObjectValue> {
  if (!path.startsWith("/v1/") && !path.startsWith("/auth/"))
    throw new Error("Invalid API target");
  const headers = new Headers({ Accept: "application/json" });
  const retryIdentity =
    (path.startsWith("/v1/owner/") ||
      path === "/v1/agents" ||
      path.endsWith("/rotate")) &&
    method !== "GET"
      ? JSON.stringify([csrf, method, path, body, match])
      : "";
  const isOwnerAgentCreation = path === "/v1/agents" && method === "POST";
  const issuesCredential =
    isOwnerAgentCreation ||
    (method === "POST" &&
      (/^\/v1\/owner\/credentials\/[^/]+\/rotate$/.test(path) ||
        /^\/v1\/owner\/agents\/[^/]+\/recover-credential$/.test(path)));
  if (isOwnerAgentCreation) {
    const previous = pendingOwnerAgentKeys.get(csrf);
    if (previous && previous.identity !== retryIdentity)
      pendingIntentKeys.delete(previous.identity);
  }
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
  if (isOwnerAgentCreation)
    pendingOwnerAgentKeys.set(csrf, {
      identity: retryIdentity,
      key: commandKey,
    });
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
  const clearRetryKey = () => {
    if (!retryIdentity) return;
    pendingIntentKeys.delete(retryIdentity);
    if (
      isOwnerAgentCreation &&
      pendingOwnerAgentKeys.get(csrf)?.key === commandKey
    )
      pendingOwnerAgentKeys.delete(csrf);
  };
  if (!response.ok && response.status < 500) clearRetryKey();
  if (response.status === 204) {
    if (issuesCredential) throw new Error("Invalid server response");
    validateResult?.({});
    clearRetryKey();
    return {};
  }
  let value: unknown;
  if (response.ok) {
    try {
      value = await response.json();
    } catch {
      // The server may have committed even when the response body was lost.
      // Keep the command key so a same-form retry recovers that exact result.
      throw new Error("Invalid server response");
    }
  } else {
    value = await response.json().catch(() => ({}));
  }
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
  if (
    issuesCredential &&
    (typeof result.id !== "string" ||
      result.id.length === 0 ||
      typeof result.token !== "string" ||
      result.token.length === 0)
  )
    throw new Error("Invalid server response");
  // A committed but unusable response is still uncertain. Resource-specific
  // validation must succeed before discarding the recoverable command key.
  validateResult?.(result);
  clearRetryKey();
  return result;
}
