import { useEffect, useState } from 'react';
import { adminApi } from '../lib/api';
import type { Analytics, VisitEvent, VisitPlace } from '../lib/types';
import { useSession } from './session';
import { Badge, ErrorNote, Panel } from './ui';

const RANGES = [7, 30, 90];

/** Visitor analytics: where and when people view the site and open the CV. */
export default function Visitors() {
  const { errorText } = useSession();
  const [days, setDays] = useState(30);
  const [data, setData] = useState<Analytics | null>(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let live = true;
    setLoading(true);
    adminApi
      .analytics(days)
      .then((d) => live && (setData(d), setError('')))
      .catch((err) => live && setError(errorText(err)))
      .finally(() => live && setLoading(false));
    return () => {
      live = false;
    };
  }, [days, errorText]);

  return (
    <section className="vis" aria-labelledby="vis-title">
      <div className="vis-head">
        <h2 id="vis-title">Visitors</h2>
        <div className="vis-range" role="group" aria-label="Date range">
          {RANGES.map((d) => (
            <button key={d} type="button" aria-pressed={d === days} className={d === days ? 'is-on' : ''} onClick={() => setDays(d)}>
              Last {d} days
            </button>
          ))}
        </div>
      </div>

      {error && <ErrorNote text={error} />}
      {!data && !error && <p className="adm-muted">Loading…</p>}
      {data && (
        <div className={loading ? 'vis-body is-loading' : 'vis-body'}>
          <p className="adm-hint">
            People only: bots and your own signed-in visits are left out. Times in {data.timezone}.
            {data.since ? ` Recording since ${new Date(data.since).toLocaleDateString(undefined, { dateStyle: 'medium' })}.` : ' Nothing recorded yet.'}
          </p>

          <div className="adm-stats">
            <Tile label="Visitors" value={data.visitors} />
            <Tile label="Site views" value={data.views} />
            <Tile label="CV opens" value={data.cv} />
            <Tile label="Bot hits filtered out" value={data.bots} dim />
          </div>

          <div className="job-two">
            <Panel title="Site views per day">
              <Columns
                data={data.daily.map((d) => ({ key: d.day, value: d.views, label: dayLabel(d.day), tick: dayTick(d.day, data.daily.length) }))}
                unit="views"
              />
            </Panel>
            <Panel title="CV opens per day">
              <Columns
                data={data.daily.map((d) => ({ key: d.day, value: d.cv, label: dayLabel(d.day), tick: dayTick(d.day, data.daily.length) }))}
                unit="CV opens"
              />
            </Panel>
          </div>

          <Panel title="When people visit (hour of day)">
            <Columns
              data={data.hours.map((h) => ({
                key: String(h.hour),
                value: h.views + h.cv,
                label: `${String(h.hour).padStart(2, '0')}:00–${String(h.hour).padStart(2, '0')}:59 · ${h.views} views, ${h.cv} CV opens`,
                tick: h.hour % 3 === 0 ? String(h.hour).padStart(2, '0') : '',
              }))}
              unit="visits"
            />
          </Panel>

          <div className="job-two">
            <Panel title="Countries">
              <Places places={data.countries} empty="No visits in this range." />
            </Panel>
            <Panel title="Cities">
              {data.hasCity || data.cities.length > 0 ? (
                <Places places={data.cities} empty="No cities recorded in this range." />
              ) : (
                <p className="adm-muted">
                  Cities need one setting in Cloudflare: <b>Rules → Settings → Managed Transforms → Add visitor location headers</b>.
                  Countries already work without it.
                </p>
              )}
            </Panel>
          </div>

          {data.referrers.length > 0 && (
            <Panel title="Where people came from">
              <Places
                places={data.referrers.map((r) => ({ code: '', name: r.host, views: r.views, cv: r.cv }))}
                empty=""
              />
            </Panel>
          )}

          <Panel title="Recent CV opens">
            {data.recentCv.length === 0 ? (
              <p className="adm-muted">No one opened the CV in this range.</p>
            ) : (
              <RecentTable events={data.recentCv} />
            )}
          </Panel>
        </div>
      )}
    </section>
  );
}

function Tile({ label, value, dim }: { label: string; value: number; dim?: boolean }) {
  return (
    <div className={`adm-stat${dim ? ' is-dim' : ''}`}>
      <span className="adm-stat-value">{value.toLocaleString()}</span>
      <span className="adm-stat-label">{label}</span>
    </div>
  );
}

// ── column chart ──────────────────────────────────────────────────────────────

type Column = { key: string; value: number; label: string; tick: string };

/** Single-series columns: one baseline, one accent, a clean top tick, a
 *  tooltip per column on hover and focus, and the numbers as a table. */
function Columns({ data, unit }: { data: Column[]; unit: string }) {
  const [hover, setHover] = useState<number | null>(null);
  const max = niceMax(Math.max(0, ...data.map((d) => d.value)));
  const total = data.reduce((n, d) => n + d.value, 0);
  const active = hover === null ? null : data[hover];

  return (
    <figure className="vis-chart">
      <div className="vis-plot" onPointerLeave={() => setHover(null)}>
        <div className="vis-axis" aria-hidden="true">
          <span>{max.toLocaleString()}</span>
          <span>0</span>
        </div>
        <div className="vis-bars" style={{ gridTemplateColumns: `repeat(${data.length}, minmax(0, 1fr))` }}>
          <span className="vis-grid" aria-hidden="true" />
          {data.map((d, i) => (
            <button
              key={d.key}
              type="button"
              className={`vis-col${hover === i ? ' is-hover' : ''}`}
              aria-label={`${d.label}: ${d.value} ${unit}`}
              onPointerEnter={() => setHover(i)}
              onFocus={() => setHover(i)}
              onBlur={() => setHover(null)}
            >
              <span className="vis-bar" style={{ height: max ? `${(d.value / max) * 100}%` : 0 }} />
            </button>
          ))}
          {active && (
            <div
              className="vis-tip"
              role="status"
              style={{ left: `${((hover! + 0.5) / data.length) * 100}%` }}
            >
              <strong>{active.value.toLocaleString()}</strong> {unit}
              <span>{active.label}</span>
            </div>
          )}
        </div>
      </div>
      <div className="vis-ticks" aria-hidden="true" style={{ gridTemplateColumns: `repeat(${data.length}, minmax(0, 1fr))` }}>
        {data.map((d) => (
          <span key={d.key}>{d.tick}</span>
        ))}
      </div>
      <details className="vis-table">
        <summary>
          {total.toLocaleString()} {unit} · show as table
        </summary>
        <table>
          <tbody>
            {data
              .filter((d) => d.value > 0)
              .map((d) => (
                <tr key={d.key}>
                  <td>{d.label}</td>
                  <td>{d.value.toLocaleString()}</td>
                </tr>
              ))}
          </tbody>
        </table>
      </details>
    </figure>
  );
}

/** The smallest of 1/2/5 × 10ⁿ at or above v, so the top tick is a clean number. */
function niceMax(v: number) {
  if (v <= 0) return 0;
  const p = 10 ** Math.floor(Math.log10(v));
  return [1, 2, 5, 10].map((m) => m * p).find((x) => x >= v)!;
}

function dayLabel(day: string) {
  return new Date(`${day}T00:00:00`).toLocaleDateString(undefined, { weekday: 'short', day: 'numeric', month: 'short' });
}

/** Sparse x labels: weekdays for a week, Mondays for a month, the 1st and
 *  15th beyond that. */
function dayTick(day: string, count: number) {
  const d = new Date(`${day}T00:00:00`);
  if (count <= 7) return d.toLocaleDateString(undefined, { weekday: 'short' });
  const show = count <= 31 ? d.getDay() === 1 : d.getDate() === 1 || d.getDate() === 15;
  return show ? d.toLocaleDateString(undefined, { day: 'numeric', month: 'short' }) : '';
}

// ── ranked places ─────────────────────────────────────────────────────────────

function Places({ places, empty }: { places: VisitPlace[]; empty: string }) {
  if (places.length === 0) return <p className="adm-muted">{empty}</p>;
  const max = Math.max(...places.map((p) => p.views + p.cv));
  return (
    <ol className="vis-places">
      {places.map((p) => (
        <li key={p.code + p.name}>
          <span className="vis-place-name">
            {p.code && p.name !== p.code && <span className="vis-flag">{flag(p.code)}</span>}
            {p.name}
          </span>
          <span className="vis-place-bar" aria-hidden="true">
            <span style={{ width: `${((p.views + p.cv) / max) * 100}%` }} />
          </span>
          <span className="vis-place-num">
            {p.views.toLocaleString()} views{p.cv > 0 && ` · ${p.cv} CV`}
          </span>
        </li>
      ))}
    </ol>
  );
}

/** "ID" → 🇮🇩 */
function flag(code: string) {
  if (!/^[A-Z]{2}$/.test(code)) return '';
  return String.fromCodePoint(...[...code].map((c) => 0x1f1e6 + c.charCodeAt(0) - 65));
}

function RecentTable({ events }: { events: VisitEvent[] }) {
  const [all, setAll] = useState(false);
  const shown = all ? events : events.slice(0, 10);
  return (
    <div className="vis-recent">
      <table>
        <thead>
          <tr>
            <th>When</th>
            <th>Where</th>
            <th>IP (masked)</th>
            <th>From</th>
            <th>Device</th>
          </tr>
        </thead>
        <tbody>
          {shown.map((e, i) => (
            <tr key={i} className={e.bot ? 'is-bot' : ''}>
              <td>{new Date(e.at).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })}</td>
              <td>
                {flag(e.countryCode)} {[e.city, e.region, e.countryCode ? e.country : ''].filter(Boolean).join(', ') || 'Unknown'}
              </td>
              <td>
                <code>{e.ip || '—'}</code>
              </td>
              <td>{e.referrer || 'Direct'}</td>
              <td>
                {e.bot ? <Badge tone="dim">Bot</Badge> : `${e.browser} · ${e.os} · ${e.device}`}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {events.length > shown.length && (
        <button type="button" className="adm-btn adm-btn--ghost vis-more" onClick={() => setAll(true)}>
          Show {events.length - shown.length} more
        </button>
      )}
    </div>
  );
}
