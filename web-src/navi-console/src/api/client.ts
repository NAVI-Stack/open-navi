import { type ZodType } from 'zod';
import { NaviApiError, normalizeError } from './errors';

export async function naviFetch<T>(
  path: string,
  schema?: ZodType<T, any, any>,
  init?: RequestInit,
): Promise<T> {
  let res: Response;
  try {
    res = await fetch(path, {
      ...init,
      credentials: 'same-origin',
    });
  } catch (err) {
    throw new NaviApiError({
      status: 0,
      message: err instanceof Error ? err.message : 'Network error',
      raw: err,
    });
  }

  if (!res.ok) {
    let body: unknown;
    try {
      body = await res.json();
    } catch {
      body = await res.text().catch(() => null);
    }
    throw new NaviApiError(normalizeError(res.status, body));
  }

  if (res.status === 204) {
    return {} as T;
  }

  const text = await res.text();
  if (!text) {
    return {} as T;
  }

  let data;
  try {
    data = JSON.parse(text);
  } catch {
    data = text;
  }

  if (schema) {
    const result = schema.safeParse(data);
    if (!result.success) {
      console.warn('Response schema validation failed:', result.error.issues);
      return data as T;
    }
    return result.data;
  }

  return data as T;
}
