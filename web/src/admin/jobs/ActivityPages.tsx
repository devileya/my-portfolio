import { useState } from 'react';
import { Link } from 'react-router-dom';
import { jobsApi } from '../../lib/api';
import { PORTAL_LABEL, type Application } from '../../lib/jobs-types';
import { useSession } from '../session';
import { Badge, ErrorNote, Loading, PageHeader, Panel, formatDate, useLoad } from '../ui';
import { Score, meta, usePoll } from './common';

const APP_FILTERS = [
  { key: '', label: 'All' },
  { key: 'submitted', label: 'Submitted' },
  { key: 'dry_run', label: 'Dry runs' },
  { key: 'needs_input', label: 'Stopped for answers' },
  { key: 'failed', label: 'Failed' },
];

const APP_TEXT: Record<Application['status'], string> = {
  running: 'Running',
  submitted: 'Submitted',
  dry_run: 'Dry run',
  needs_input: 'Needs answers',
  external: 'External',
  failed: 'Failed',
};

export function ApplicationsPage() {
  const [status, setStatus] = useState('');
  return (
    <>
      <PageHeader title="Applications" hint="Every attempt the worker made, with a screenshot of where it stopped." />
      <nav className="job-tabs" aria-label="Status">
        {APP_FILTERS.map((f) => (
          <button key={f.key} type="button" className={f.key === status ? 'is-on' : ''} onClick={() => setStatus(f.key)}>
            {f.label}
          </button>
        ))}
      </nav>
      <ApplicationList key={status} status={status} />
    </>
  );
}

function ApplicationList({ status }: { status: string }) {
  const { data, error, reload } = useLoad(() => jobsApi.applications(status));
  return (
    <>
      {error && <ErrorNote text={error} onRetry={reload} />}
      {!data && !error && <Loading />}
      {data && data.length === 0 && (
        <Panel>
          <p className="adm-muted">No applications yet. Approve jobs in the review queue and the worker takes it from there.</p>
        </Panel>
      )}
      {data && data.length > 0 && (
        <ul className="adm-rows-list job-list">
          {data.map((a) => (
            <li key={a.id}>
              <Score value={a.listing?.score ?? null} />
              <div className="adm-thumb">{a.screenshotUrl ? <img src={a.screenshotUrl} alt="" loading="lazy" /> : null}</div>
              <div className="adm-row-main">
                {a.listing ? (
                  <Link to={`/jobs/listings/${a.listingId}`} className="adm-row-title">
                    {a.listing.title}
                  </Link>
                ) : (
                  <span className="adm-row-title">Job #{a.listingId}</span>
                )}
                <div className="adm-row-sub">
                  <Badge tone={a.status === 'failed' || a.status === 'needs_input' ? 'hot' : a.status === 'dry_run' ? 'dim' : 'plain'}>
                    {APP_TEXT[a.status]}
                  </Badge>
                  <span>{meta([a.listing?.company, a.listing && PORTAL_LABEL[a.listing.portal]])}</span>
                  <span className="adm-muted">{formatDate(a.startedAt)}</span>
                </div>
                {a.error && <p className="job-reason">{a.error}</p>}
              </div>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}

export function ActivityPage() {
  const { errorText, notify } = useSession();
  const { data, setData, error, reload } = useLoad(jobsApi.tasks);
  const [open, setOpen] = useState<number | null>(null);
  usePoll(() => jobsApi.tasks().then(setData).catch(() => {}), 10000);

  async function cancel(id: number) {
    try {
      await jobsApi.cancelTask(id);
      reload();
    } catch (err) {
      notify(errorText(err), 'error');
    }
  }

  return (
    <>
      <PageHeader title="Activity" hint="Searches, sign-ins and session checks the worker ran, newest first." />
      {error && <ErrorNote text={error} onRetry={reload} />}
      {!data && !error && <Loading />}
      {data && data.length === 0 && (
        <Panel>
          <p className="adm-muted">Nothing has run yet.</p>
        </Panel>
      )}
      {data && data.length > 0 && (
        <ul className="adm-rows-list">
          {data.map((t) => (
            <li key={t.id} className="job-task">
              <div className="job-task-head">
                <span className={`job-dot job-dot--${t.status}`} aria-label={t.status} />
                <div className="adm-row-main">
                  <span className="adm-row-title">
                    {t.kind === 'search' ? `Search “${t.searchName || 'deleted'}”` : t.kind === 'login' ? 'Sign-in' : 'Session check'} ·{' '}
                    {t.portal ? PORTAL_LABEL[t.portal] : '—'}
                  </span>
                  <div className="adm-row-sub">
                    <span>{t.status}</span>
                    <span>{t.trigger}</span>
                    {t.kind === 'search' && t.status === 'done' && (
                      <span>
                        {t.result.found ?? 0} found · {t.result.new ?? 0} new · {t.result.queued ?? 0} queued · {t.result.approved ?? 0} auto-approved ·{' '}
                        {t.result.filtered ?? 0} filtered
                      </span>
                    )}
                    <span className="adm-muted">{formatDate(t.finishedAt ?? t.startedAt ?? t.createdAt)}</span>
                  </div>
                  {t.error && <p className="adm-error-text">{t.error}</p>}
                </div>
                <div className="adm-row-actions">
                  {t.log && (
                    <button type="button" className="adm-btn adm-btn--ghost" aria-expanded={open === t.id} onClick={() => setOpen(open === t.id ? null : t.id)}>
                      {open === t.id ? 'Hide log' : 'Log'}
                    </button>
                  )}
                  {t.status === 'pending' && (
                    <button type="button" className="adm-btn adm-btn--ghost" onClick={() => cancel(t.id)}>
                      Cancel
                    </button>
                  )}
                </div>
              </div>
              {open === t.id && <pre className="job-log">{t.log}</pre>}
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
