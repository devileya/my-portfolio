import type {
  Admin,
  Analytics,
  Experience,
  Message,
  Profile,
  Project,
  Site,
  Skill,
  Stats,
  UploadedObject,
} from './types';
import type {
  AIProvider,
  Application,
  JobAccount,
  JobSearch,
  JobSettings,
  JobTask,
  JobsOverview,
  Listing,
  Portal,
  SavedAnswer,
} from './jobs-types';

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
  }
}

async function request<T>(method: string, url: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { 'X-Requested-With': 'portfolio' };
  let payload: BodyInit | undefined;
  if (body instanceof FormData) {
    payload = body;
  } else if (body !== undefined) {
    headers['Content-Type'] = 'application/json';
    payload = JSON.stringify(body);
  }

  let res: Response;
  try {
    res = await fetch(url, { method, headers, body: payload, credentials: 'same-origin' });
  } catch {
    throw new ApiError('Can’t reach the server. Check your connection and try again.', 0);
  }

  if (res.status === 204) return undefined as T;
  const data = await res.json().catch(() => null);
  if (!res.ok) {
    throw new ApiError(data?.error ?? `Request failed (${res.status}).`, res.status);
  }
  return data as T;
}

const get = <T>(url: string) => request<T>('GET', url);
const post = <T>(url: string, body?: unknown) => request<T>('POST', url, body);
const put = <T>(url: string, body?: unknown) => request<T>('PUT', url, body);
const patch = <T>(url: string, body?: unknown) => request<T>('PATCH', url, body);
const del = <T>(url: string) => request<T>('DELETE', url);

function upload<T>(url: string, file: File) {
  const form = new FormData();
  form.append('file', file);
  return post<T>(url, form);
}

// ── public ────────────────────────────────────────────────────────────────────

export const publicApi = {
  site: () => get<Site>('/api/site'),
  sendMessage: (input: { name: string; email: string; message: string; website: string }) =>
    post<{ ok: boolean }>('/api/messages', input),
};

// ── dashboard ─────────────────────────────────────────────────────────────────

function crud<T extends { id: number }>(base: string) {
  return {
    list: () => get<T[]>(base),
    create: (v: Omit<T, 'id'>) => post<T>(base, v),
    update: (v: T) => put<T>(`${base}/${v.id}`, v),
    remove: (id: number) => del<void>(`${base}/${id}`),
    reorder: (ids: number[]) => post<void>(`${base}/reorder`, { ids }),
  };
}

export const adminApi = {
  login: (email: string, password: string) => post<Admin>('/api/admin/login', { email, password }),
  logout: () => post<void>('/api/admin/logout'),
  me: () => get<Admin>('/api/admin/me'),
  changePassword: (current: string, next: string) =>
    put<void>('/api/admin/password', { current, next }),
  stats: () => get<Stats>('/api/admin/stats'),
  analytics: (days: number) =>
    get<Analytics>(`/api/admin/analytics?days=${days}&tz=${encodeURIComponent(Intl.DateTimeFormat().resolvedOptions().timeZone)}`),

  profile: () => get<Profile>('/api/admin/profile'),
  saveProfile: (p: Profile) => put<Profile>('/api/admin/profile', p),

  uploadImage: (file: File) => upload<UploadedObject>('/api/admin/uploads', file),
  uploadCV: (file: File) => upload<Profile>('/api/admin/cv', file),
  deleteCV: () => del<Profile>('/api/admin/cv'),

  projects: crud<Project>('/api/admin/projects'),
  experiences: crud<Experience>('/api/admin/experiences'),
  skills: crud<Skill>('/api/admin/skills'),

  messages: () => get<Message[]>('/api/admin/messages'),
  markMessage: (id: number, read: boolean) => patch<void>(`/api/admin/messages/${id}`, { read }),
  deleteMessage: (id: number) => del<void>(`/api/admin/messages/${id}`),
};

// ── job hunter ────────────────────────────────────────────────────────────────

const J = '/api/admin/jobs';

export interface ListingQuery {
  status?: string;
  portal?: string;
  search?: number;
  q?: string;
  limit?: number;
  offset?: number;
}

function query(q: ListingQuery) {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(q)) if (v !== undefined && v !== '' && v !== 0) p.set(k, String(v));
  const s = p.toString();
  return s ? `?${s}` : '';
}

// Only the editable fields: timestamps and run times belong to the server.
function searchBody(s: Omit<JobSearch, 'id'> & { id?: number }) {
  const { id: _id, lastRunAt: _l, nextRunAt: _n, createdAt: _c, updatedAt: _u, ...body } = s;
  return body;
}

export const jobsApi = {
  overview: () => get<JobsOverview>(`${J}/overview`),
  settings: () => get<JobSettings>(`${J}/settings`),
  saveSettings: (s: JobSettings) => put<JobSettings>(`${J}/settings`, s),
  pause: (paused: boolean) => post<void>(`${J}/pause`, { paused }),
  testTelegram: () => post<void>(`${J}/telegram/test`),
  saveTelegramToken: (token: string) => put<JobSettings>(`${J}/telegram/token`, { token }),
  telegramChats: () => get<{ id: number; name: string }[]>(`${J}/telegram/chats`),
  saveTelegramChat: (chatId: number, chatName: string, poll: boolean) =>
    put<JobSettings>(`${J}/telegram/chat`, { chatId, chatName, poll }),
  clearTelegram: () => del<JobSettings>(`${J}/telegram`),

  providers: () => get<AIProvider[]>(`${J}/providers`),
  createProvider: (p: Partial<AIProvider>) => post<AIProvider>(`${J}/providers`, p),
  updateProvider: (p: Partial<AIProvider> & { id: number }) => put<AIProvider>(`${J}/providers/${p.id}`, p),
  deleteProvider: (id: number) => del<void>(`${J}/providers/${id}`),
  testProvider: (id: number) => post<{ ok: boolean; ms: number }>(`${J}/providers/${id}/test`),
  providerModels: (id: number) => get<string[]>(`${J}/providers/${id}/models`),

  accounts: () => get<JobAccount[]>(`${J}/accounts`),
  setAccountEnabled: (portal: Portal, enabled: boolean) => put<JobAccount[]>(`${J}/accounts/${portal}`, { enabled }),
  importCookies: (portal: Portal, json: string) =>
    request<JobAccount[]>('POST', `${J}/accounts/${portal}/cookies`, JSON.parse(json)),
  clearCookies: (portal: Portal) => del<JobAccount[]>(`${J}/accounts/${portal}/cookies`),
  login: (portal: Portal) => post<void>(`${J}/accounts/${portal}/login`),
  check: (portal: Portal) => post<void>(`${J}/accounts/${portal}/check`),

  searches: () => get<JobSearch[]>(`${J}/searches`),
  createSearch: (s: Omit<JobSearch, 'id'>) => post<JobSearch>(`${J}/searches`, searchBody(s)),
  updateSearch: (s: JobSearch) => put<JobSearch>(`${J}/searches/${s.id}`, searchBody(s)),
  deleteSearch: (id: number) => del<void>(`${J}/searches/${id}`),
  runSearch: (id: number) => post<{ queued: number }>(`${J}/searches/${id}/run`),

  listings: (q: ListingQuery) => get<{ items: Listing[]; total: number }>(`${J}/listings${query(q)}`),
  listing: (id: number) => get<{ listing: Listing; applications: Application[] }>(`${J}/listings/${id}`),
  review: (id: number, approve: boolean) => post<Listing>(`${J}/listings/${id}/review`, { approve }),
  bulkReview: (ids: number[], approve: boolean) => post<{ updated: number }>(`${J}/listings/review`, { ids, approve }),
  answer: (id: number, answers: { question: string; answer: string }[]) =>
    post<{ requeued: boolean }>(`${J}/listings/${id}/answers`, { answers }),
  markApplied: (id: number) => post<{ listing: Listing; applications: Application[] }>(`${J}/listings/${id}/applied`),

  applications: (status?: string) => get<Application[]>(`${J}/applications${status ? `?status=${status}` : ''}`),

  answers: () => get<SavedAnswer[]>(`${J}/answers`),
  createAnswer: (a: { question: string; answer: string }) => post<SavedAnswer>(`${J}/answers`, a),
  updateAnswer: (a: SavedAnswer) => put<SavedAnswer>(`${J}/answers/${a.id}`, a),
  deleteAnswer: (id: number) => del<void>(`${J}/answers/${id}`),

  tasks: () => get<JobTask[]>(`${J}/tasks`),
  cancelTask: (id: number) => del<void>(`${J}/tasks/${id}`),
};
