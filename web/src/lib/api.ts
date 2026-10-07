import type {
  Admin,
  Experience,
  Message,
  Profile,
  Project,
  Site,
  Skill,
  Stats,
  UploadedObject,
} from './types';

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
