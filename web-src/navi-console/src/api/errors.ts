export interface NormalizedError {
  status: number;
  code?: string;
  message: string;
  details?: unknown;
  raw?: unknown;
}

export class NaviApiError extends Error {
  readonly status: number;
  readonly code?: string;
  readonly details?: unknown;
  readonly raw?: unknown;

  constructor(err: NormalizedError) {
    super(err.message);
    this.name = 'NaviApiError';
    this.status = err.status;
    this.code = err.code;
    this.details = err.details;
    this.raw = err.raw;
  }
}

export function normalizeError(status: number, body: unknown): NormalizedError {
  if (body && typeof body === 'object' && 'error' in body) {
    const errField = (body as Record<string, unknown>).error;

    if (typeof errField === 'string') {
      return { status, message: errField, raw: body };
    }

    if (errField && typeof errField === 'object') {
      const obj = errField as Record<string, unknown>;
      return {
        status,
        code: typeof obj.code === 'string' ? obj.code : undefined,
        message: typeof obj.message === 'string' ? obj.message : `HTTP ${status}`,
        details: obj.details,
        raw: body,
      };
    }
  }

  if (body && typeof body === 'object' && 'message' in body) {
    const msg = (body as Record<string, unknown>).message;
    if (typeof msg === 'string') {
      return { status, message: msg, raw: body };
    }
  }

  return { status, message: `HTTP ${status}`, raw: body };
}
