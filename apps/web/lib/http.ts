export type ApiEnvelope<T> = {
  data?: T;
  error?: { code?: string; message?: string };
};

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
