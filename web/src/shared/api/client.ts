export const isDemo = typeof window === 'undefined' || import.meta.env?.VITE_DEMO_MODE === 'true';
export interface SessionScope { actor: string; permissions: string; companyId: string; epoch: string; name: string; companyName: string }
let scope: SessionScope = { actor: '', permissions: '', companyId: '', epoch: '', name: '', companyName: '' };
export function configureScope(value: SessionScope) { scope = value; }
export function apiScope() { return scope; }
export class ApiError extends Error {
  code: string; requestId: string; status: number;
  constructor(code: string, message: string, requestId: string, status: number) { super(message); this.code = code; this.requestId = requestId; this.status = status; }
}
export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body && typeof init.body === 'string') headers.set('Content-Type', 'application/json');
  let response: Response;
  try { response = await fetch(`/api/v1${path}`, { ...init, headers, credentials: 'same-origin' }); } catch (error) {
    if (init.signal?.aborted) throw error;
    throw new ApiError('NETWORK_ERROR', 'Không nhận được phản hồi. Giữ nguyên nội dung và thử lại để kiểm tra kết quả của thao tác.', '', 0);
  }
  if (!response.ok) {
    const body = await response.json().catch(() => ({ code: 'NETWORK_ERROR', message: 'Không nhận được phản hồi hợp lệ từ máy chủ.' })) as { code: string; message: string; request_id?: string };
    if (path !== '/auth/login' && (body.code === 'UNAUTHENTICATED' || body.code === 'DATASET_CHANGED')) window.dispatchEvent(new CustomEvent('oanh:session-changed', { detail: body.code }));
    throw new ApiError(body.code, body.message, body.request_id ?? '', response.status);
  }
  return response.json() as Promise<T>;
}
const pendingIntents = new Map<string, string>();
export async function command<T>(path: string, payload: unknown, epoch = scope.epoch): Promise<T> {
  const body = JSON.stringify(payload);
  const bytes = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(`${scope.actor}:${epoch}:${path}:${body}`));
  const hash = [...new Uint8Array(bytes)].map(value => value.toString(16).padStart(2, '0')).join('');
  let key = pendingIntents.get(hash);
  if (!key) { key = crypto.randomUUID(); pendingIntents.set(hash, key); }
  const result = await api<T>(path, { method: 'POST', body, headers: { 'Idempotency-Key': key, 'X-Dataset-Epoch': epoch } });
  pendingIntents.delete(hash);
  return result;
}
export interface Session { needsSetup: boolean; user?: { id: string; username: string; name: string; role: string; active: boolean }; context?: { companyId: string; companyName: string; epoch: string; revision: number; currency: string; scale: number; regime: string } }
export async function loadSession(signal?: AbortSignal): Promise<Session> {
  const session = await api<Session>('/session', { signal });
  if (session.user && session.context) configureScope({ actor: session.user.id, permissions: session.user.role, name: session.user.name, companyId: session.context.companyId, companyName: session.context.companyName, epoch: session.context.epoch });
  return session;
}

export async function downloadApi(path: string, filename: string, init: RequestInit = {}): Promise<void> {
  const response = await fetch(`/api/v1${path}`, { ...init, credentials: 'same-origin' });
  if (!response.ok) { const body = await response.json() as { message: string }; throw new Error(body.message); }
  const url = URL.createObjectURL(await response.blob());
  const link = document.createElement('a'); link.href = url; link.download = filename; link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
