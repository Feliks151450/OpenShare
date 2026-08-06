export interface RequestOptions extends Omit<RequestInit, "body"> {
  body?: BodyInit | Record<string, unknown> | null;
}

export class HttpError extends Error {
  readonly status: number;
  readonly payload: unknown;

  constructor(message: string, status: number, payload: unknown) {
    super(message);
    this.name = "HttpError";
    this.status = status;
    this.payload = payload;
  }
}

const defaultHeaders = {
  Accept: "application/json",
} satisfies HeadersInit;

/** 命中以下前缀的请求会被附加 X-OpenShare-Guest-Key 头（密钥头）。 */
const BROWSE_ENDPOINT_PREFIXES = [
  "/public/folders",
  "/public/files",
  "/public/search",
  "/public/resolve-custom-path",
  "/public/announcements",
  "/public/file-tags",
  "/public/guest-access/validate",
];

/** 以上前缀下不走密钥校验的端点（仅以 /public/files 为根的下载分支）。 */
const DOWNLOAD_ENDPOINT_PATTERNS = [
  /\/public\/files\/[^/]+\/download(\/|$)/,
  /\/public\/files\/[^/]+\/netcdf-dump/,
  /\/public\/files\/batch-download/,
  /\/public\/resources\/batch-download/,
  /\/public\/folders\/[^/]+\/download(\/|$)/,
];

interface GuestKeyAttachable {
  headers: Headers;
  url?: string;
}

function attachGuestAccessKey(target: GuestKeyAttachable, path: string) {
  let keyFromStorage = "";
  try {
    // 仅在浏览器环境访问 localStorage
    if (typeof window !== "undefined") {
      keyFromStorage = window.localStorage.getItem("openshare-guest-key") ?? "";
    }
  } catch {
    keyFromStorage = "";
  }
  if (!keyFromStorage) return;
  const normalized = path.startsWith("/") ? path : `/${path}`;
  const isBrowse = BROWSE_ENDPOINT_PREFIXES.some((prefix) =>
    normalized === prefix || normalized.startsWith(`${prefix}/`),
  );
  if (!isBrowse) return;
  if (DOWNLOAD_ENDPOINT_PATTERNS.some((pattern) => pattern.test(normalized))) return;
  target.headers.set("X-OpenShare-Guest-Key", keyFromStorage);
}

export class HttpClient {
  constructor(private readonly baseURL = "/api") {}

  async request<T>(path: string, options: RequestOptions = {}): Promise<T> {
    const headers = new Headers(defaultHeaders);

    if (options.headers) {
      new Headers(options.headers).forEach((value, key) => headers.set(key, value));
    }

    attachGuestAccessKey({ headers }, path);

    const response = await fetch(this.resolveURL(path), {
      ...options,
      headers,
      credentials: "include",
      body: normalizeBody(options.body, headers),
    });

    const payload = await parsePayload(response);

    if (!response.ok) {
      throw new HttpError(response.statusText || "Request failed", response.status, payload);
    }

    return payload as T;
  }

  get<T>(path: string, options?: RequestOptions) {
    return this.request<T>(path, { ...options, method: "GET" });
  }

  post<T>(path: string, body?: RequestOptions["body"], options?: RequestOptions) {
    return this.request<T>(path, { ...options, method: "POST", body });
  }

  put<T>(path: string, body?: RequestOptions["body"], options?: RequestOptions) {
    return this.request<T>(path, { ...options, method: "PUT", body });
  }

  patch<T>(path: string, body?: RequestOptions["body"], options?: RequestOptions) {
    return this.request<T>(path, { ...options, method: "PATCH", body });
  }

  delete<T>(path: string, body?: RequestOptions["body"], options?: RequestOptions) {
    return this.request<T>(path, { ...options, method: "DELETE", body });
  }

  private resolveURL(path: string) {
    if (/^https?:\/\//.test(path)) {
      return path;
    }
    return `${this.baseURL}${path.startsWith("/") ? path : `/${path}`}`;
  }
}

function normalizeBody(body: RequestOptions["body"], headers: Headers): BodyInit | null | undefined {
  if (body == null) {
    return body;
  }

  if (body instanceof FormData || body instanceof URLSearchParams || typeof body === "string" || body instanceof Blob) {
    return body;
  }

  headers.set("Content-Type", "application/json");
  return JSON.stringify(body);
}

async function parsePayload(response: Response): Promise<unknown> {
  if (response.status === 204) {
    return null;
  }

  const contentType = response.headers.get("content-type") ?? "";

  if (contentType.includes("application/json")) {
    return response.json();
  }

  return response.text();
}

export const httpClient = new HttpClient(import.meta.env.VITE_API_BASE_URL ?? "/api");
