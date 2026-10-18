import { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { jobsApi } from '../../lib/api';
import { PORTALS, PORTAL_LABEL, label, type Listing } from '../../lib/jobs-types';
import { useSession } from '../session';
import { ErrorNote, Loading, PageHeader, Panel } from '../ui';
import { Score, StatusBadge, ago, meta } from './common';

const TABS = [
  { key: 'queued', label: 'To review', status: 'queued' },
  { key: 'approved', label: 'Approved', status: 'approved,applying' },
  { key: 'needs_input', label: 'Need answers', status: 'needs_input' },
  { key: 'applied', label: 'Applied', status: 'applied,dry_run' },
  { key: 'external', label: 'Apply on site', status: 'external' },
  { key: 'failed', label: 'Failed', status: 'failed' },
  { key: 'filtered', label: 'Filtered & skipped', status: 'filtered,skipped,new' },
  { key: 'all', label: 'All', status: '' },
];

const PAGE = 50;

export default function QueuePage() {
  const { errorText, notify } = useSession();
  const [params, setParams] = useSearchParams();
  const tab = TABS.find((t) => t.key === params.get('tab')) ?? TABS[0];
  const [portal, setPortal] = useState('');
  const [q, setQ] = useState('');
  const [query, setQuery] = useState('');
  const [items, setItems] = useState<Listing[] | null>(null);
  const [total, setTotal] = useState(0);
  const [error, setError] = useState('');
  const [selected, setSelected] = useState<number[]>([]);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    const t = window.setTimeout(() => setQuery(q), 300);
    return () => window.clearTimeout(t);
  }, [q]);

  useEffect(() => {
    let live = true;
    setItems(null);
    setSelected([]);
    setError('');
    jobsApi
      .listings({ status: tab.status, portal, q: query, limit: PAGE })
      .then((r) => {
        if (!live) return;
        setItems(r.items);
        setTotal(r.total);
      })
      .catch((err) => live && setError(errorText(err)));
    return () => {
      live = false;
    };
  }, [tab.status, portal, query, errorText]);

  async function more() {
    if (!items) return;
    try {
      const r = await jobsApi.listings({ status: tab.status, portal, q: query, limit: PAGE, offset: items.length });
      setItems([...items, ...r.items]);
      setTotal(r.total);
    } catch (err) {
      notify(errorText(err), 'error');
    }
  }

  async function review(ids: number[], approve: boolean) {
    setBusy(true);
    try {
      if (ids.length === 1) await jobsApi.review(ids[0], approve);
      else await jobsApi.bulkReview(ids, approve);
      notify(approve ? `Approved ${ids.length}: the worker applies in its next window` : `Skipped ${ids.length}`);
      if (tab.key === 'queued' || tab.key === 'failed' || tab.key === 'external') {
        setItems((items ?? []).filter((l) => !ids.includes(l.id)));
        setTotal((n) => n - ids.length);
      } else {
        setItems((items ?? []).map((l) => (ids.includes(l.id) ? { ...l, status: approve ? 'approved' : 'skipped' } : l)));
      }
      setSelected([]);
    } catch (err) {
      notify(errorText(err), 'error');
    } finally {
      setBusy(false);
    }
  }

  const allOn = !!items && items.length > 0 && selected.length === items.length;
  const reviewable = (l: Listing) => !['applied', 'applying'].includes(l.status);

  return (
    <>
      <PageHeader title="Review queue" hint="Approve the jobs you want; the worker applies at a human pace within your window and caps." />

      <nav className="job-tabs" aria-label="Status">
        {TABS.map((t) => (
          <button
            key={t.key}
            type="button"
            className={t.key === tab.key ? 'is-on' : ''}
            aria-current={t.key === tab.key ? 'page' : undefined}
            onClick={() => setParams(t.key === 'queued' ? {} : { tab: t.key })}
          >
            {t.label}
          </button>
        ))}
      </nav>

      <div className="job-toolbar">
        <input type="search" placeholder="Filter by title, company, location" aria-label="Filter" value={q} onChange={(e) => setQ(e.target.value)} />
        <select aria-label="Portal" value={portal} onChange={(e) => setPortal(e.target.value)}>
          <option value="">All portals</option>
          {PORTALS.map((p) => (
            <option key={p} value={p}>
              {PORTAL_LABEL[p]}
            </option>
          ))}
        </select>
        {items && items.length > 0 && (
          <label className="job-select-all">
            <input type="checkbox" checked={allOn} onChange={() => setSelected(allOn ? [] : items.filter(reviewable).map((l) => l.id))} />
            Select all
          </label>
        )}
        {selected.length > 0 && (
          <span className="adm-row-actions">
            <span className="adm-muted">{selected.length} selected</span>
            <button type="button" className="adm-btn adm-btn--primary" disabled={busy} onClick={() => review(selected, true)}>
              Approve
            </button>
            <button type="button" className="adm-btn adm-btn--ghost" disabled={busy} onClick={() => review(selected, false)}>
              Skip
            </button>
          </span>
        )}
      </div>

      {error && <ErrorNote text={error} />}
      {!items && !error && <Loading />}
      {items && items.length === 0 && (
        <Panel>
          <p className="adm-muted">
            {tab.key === 'queued'
              ? 'Nothing waiting for review. Matches appear here after each scheduled search.'
              : 'No jobs in this list.'}
          </p>
        </Panel>
      )}
      {items && items.length > 0 && (
        <ul className="adm-rows-list job-list">
          {items.map((l) => (
            <li key={l.id}>
              <input
                type="checkbox"
                aria-label={`Select ${l.title}`}
                disabled={!reviewable(l)}
                checked={selected.includes(l.id)}
                onChange={(e) => setSelected(e.target.checked ? [...selected, l.id] : selected.filter((x) => x !== l.id))}
              />
              <Score value={l.score} />
              <div className="adm-row-main">
                <Link to={`/jobs/listings/${l.id}`} className="adm-row-title">
                  {l.title}
                </Link>
                <div className="adm-row-sub">
                  <span>{l.company || 'Unknown company'}</span>
                  <span>{meta([PORTAL_LABEL[l.portal], l.workMode && label(l.workMode), l.location, l.salary])}</span>
                  {tab.key === 'all' && <StatusBadge status={l.status} />}
                  {l.easyApply === false && <span className="adm-muted">external apply</span>}
                  <span className="adm-muted">found {ago(l.foundAt)}</span>
                </div>
                {(l.scoreReason || l.statusNote) && <p className="job-reason">{l.statusNote || l.scoreReason}</p>}
              </div>
              <div className="adm-row-actions">
                <a href={l.url} target="_blank" rel="noopener noreferrer" className="adm-btn adm-btn--ghost">
                  Open ↗
                </a>
                {reviewable(l) && l.status !== 'approved' && (
                  <button type="button" className="adm-btn adm-btn--primary" disabled={busy} onClick={() => review([l.id], true)}>
                    {l.status === 'failed' ? 'Retry' : 'Approve'}
                  </button>
                )}
                {reviewable(l) && l.status !== 'skipped' && l.status !== 'filtered' && (
                  <button type="button" className="adm-btn adm-btn--ghost" disabled={busy} onClick={() => review([l.id], false)}>
                    Skip
                  </button>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}
      {items && items.length < total && (
        <p className="job-more">
          <button type="button" className="adm-btn adm-btn--ghost" onClick={more}>
            Show more ({total - items.length} left)
          </button>
        </p>
      )}
    </>
  );
}
