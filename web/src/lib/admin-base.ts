// The Go server injects <meta name="pf-admin-base"> only into pages served
// under ADMIN_PATH, so the dashboard's URL never ships in the public bundle.
// In `vite dev` there is no server-side injection; /__admin stands in.
const DEV_BASE = '/__admin';

export function adminBase(): string | null {
  const meta = document.querySelector<HTMLMetaElement>('meta[name="pf-admin-base"]');
  if (meta?.content) return meta.content;
  if (import.meta.env.DEV && (location.pathname === DEV_BASE || location.pathname.startsWith(`${DEV_BASE}/`))) {
    return DEV_BASE;
  }
  return null;
}
