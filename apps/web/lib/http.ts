export type ApiEnvelope<T> = {
  data?: T;
  error?: { code?: string; message?: string };
};

function sharePendingGet(path: string) {
  let pending: Promise<Response> | undefined;

  return async function () {
    // Share overlapping reads only. A later read must observe the current session or service state.
    pending ??= fetch(path, { cache: "no-store" }).finally(() => {
      pending = undefined;
    });
    return (await pending).clone();
  };
}

export const fetchSession = sharePendingGet("/api/v1/session");
export const fetchReadiness = sharePendingGet("/readyz");

export async function readApiEnvelope<T>(
  response: Response,
  fallbackError: string,
): Promise<ApiEnvelope<T>> {
  if (response.status === 204) return {};
  const contentType = response.headers.get("content-type") ?? "";
  if (!contentType.includes("application/json")) {
    throw new Error(fallbackError);
  }
  try {
    return (await response.json()) as ApiEnvelope<T>;
  } catch {
    throw new Error(fallbackError);
  }
}
