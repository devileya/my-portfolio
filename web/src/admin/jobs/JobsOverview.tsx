import { useState } from 'react';
import { Link } from 'react-router-dom';
import { jobsApi } from '../../lib/api';
import { PORTALS, PORTAL_LABEL, type JobsOverview as Overview } from '../../lib/jobs-types';
import { useSession } from '../session';
import { ErrorNote, Loading, PageHeader, Panel, formatDate, useLoad } from '../ui';
import { AccountBadge, Score, ago, meta, usePoll } from './common';

export default function JobsOverview() {
  const { errorText, notify } = useSession();
  const { data, setData, error, reload } = useLoad(jobsApi.overview);
  const [busy, setBusy] = useState(false);
  usePoll(() => jobsApi.overview().then(setData).catch(() => {}), 15000);

  async function togglePause(o: Overview) {
    setBusy(true);
    try {
      await jobsApi.pause(!o.settings.paused);
      notify(o.settings.paused ? 'Job Hunter resumed' : 'Job Hunter paused');
      reload();
    } catch (err) {
      notify(errorText(err), 'error');
    } finally {
      setBusy(false);
    }
  }

  if (error && !data) return <ErrorNote text={error} onRetry={reload} />;
  if (!data) return <Loading />;
  const o = data;
  const s = o.settings;
  const appliedToday = PORTALS.reduce((n, p) => n + (o.appliedToday[p] ?? 0), 0);
  const workerFresh = s.workerSeenAt && Date.now() - new Date(s.workerSeenAt).getTime() < 2 * 60 * 1000;
  const connected = o.accounts.filter((a) => a.enabled && a.status === 'connected').length;

  const setup = [
    { done: o.secretsReady, text: 'Set JOBS_SECRET_KEY in the server .env', to: '' },
    { done: o.providers > 0 && s.scoreProviderId !== null, text: 'Add an AI provider (Anthropic, OpenAI, Vikey…)', to: '/jobs/settings' },
    { done: connected > 0, text: 'Connect at least one portal account', to: '/jobs/accounts' },
    { done: o.activeSearches > 0, text: 'Create a saved search', to: '/jobs/searches/new' },
    { done: o.telegramReady, text: 'Connect a Telegram bot for notifications', to: '/jobs/settings' },
  ];

  return (
    <>
      <PageHeader
        title="Job Hunter"
        hint="Saved searches run on schedule; matches are scored against your profile and wait for review."
        actions={
          <>
            <button type="button" className="adm-btn adm-btn--ghost" disabled={busy} onClick={() => togglePause(o)}>
              {s.paused ? 'Resume' : 'Pause all'}
            </button>
            <Link to="/jobs/queue" className="adm-btn adm-btn--primary">
              Review queue
            </Link>
          </>
        }
      />

      {s.paused && (
        <div className="adm-warn" role="note">
          <strong>Paused.</strong> No searches or applications run until you resume.
        </div>
      )}
      {!s.paused && s.dryRun && (
        <div className="adm-warn" role="note">
          <strong>Dry run is on.</strong> The worker fills application forms but never presses the final Submit. Check a few
          results under <Link to="/jobs/applications">Applications</Link>, then turn it off in{' '}
          <Link to="/jobs/settings">Settings</Link>.
        </div>
      )}

      <div className="adm-stats">
        <Stat label="To review" value={o.byStatus.queued ?? 0} to="/jobs/queue" hot={(o.byStatus.queued ?? 0) > 0} />
        <Stat label="Approved, waiting" value={(o.byStatus.approved ?? 0) + (o.byStatus.applying ?? 0)} to="/jobs/queue?tab=approved" />
        <Stat label="Need your answers" value={o.byStatus.needs_input ?? 0} to="/jobs/queue?tab=needs_input" hot={(o.byStatus.needs_input ?? 0) > 0} />
        <Stat label={`Applied today · ${o.appliedWeek} this week`} value={appliedToday} to="/jobs/applications" />
      </div>

      {setup.some((x) => !x.done) && (
        <Panel title="Setup">
          <ul className="job-checklist">
            {setup.map((x) => (
              <li key={x.text} className={x.done ? 'is-done' : ''}>
                <span aria-hidden="true">{x.done ? '✓' : '○'}</span>
                {x.to && !x.done ? <Link to={x.to}>{x.text}</Link> : x.text}
              </li>
            ))}
          </ul>
        </Panel>
      )}

      <div className="job-two">
        <Panel title="Worker">
          <dl className="adm-meta">
            <dt>State</dt>
            <dd>
              {workerFresh ? s.workerState || 'idle' : s.workerSeenAt ? 'offline' : 'never started'}
              {s.workerName && <span className="adm-muted"> on {s.workerName}</span>}
            </dd>
            <dt>Last seen</dt>
            <dd>{ago(s.workerSeenAt)}</dd>
            <dt>Mode</dt>
            <dd>{o.workerMode === 'embedded' ? 'Inside the server' : 'Separate `portfolio worker` process'}</dd>
            <dt>Window</dt>
            <dd>
              Applies {String(s.applyStartHour).padStart(2, '0')}:00–{String(s.applyEndHour).padStart(2, '0')}:00 {s.timezone}, every{' '}
              {s.minGapMinutes}–{s.maxGapMinutes} min per portal
            </dd>
            <dt>Tasks waiting</dt>
            <dd>
              <Link to="/jobs/activity">{o.pendingTasks}</Link>
            </dd>
          </dl>
          {!workerFresh && (
            <p className="adm-hint">
              The worker reports every few seconds while it runs. Start the server with JOBS_WORKER=embedded, or run{' '}
              <code>portfolio worker</code> on the machine with Chrome.
            </p>
          )}
        </Panel>

        <Panel title="Accounts" actions={<Link to="/jobs/accounts">Manage</Link>}>
          <ul className="job-acct-list">
            {o.accounts.map((a) => (
              <li key={a.portal}>
                <span>{PORTAL_LABEL[a.portal]}</span>
                <AccountBadge status={a.status} enabled={a.enabled} />
                <span className="adm-muted">
                  {o.appliedToday[a.portal] ?? 0}/{s.portalCaps[a.portal] ?? '∞'} today
                </span>
              </li>
            ))}
          </ul>
        </Panel>
      </div>

      <Panel title="Best matches waiting" actions={<Link to="/jobs/queue">All</Link>}>
        {o.topQueued.length === 0 ? (
          <p className="adm-muted">Nothing to review. New matches appear after each scheduled search.</p>
        ) : (
          <ul className="job-mini-list">
            {o.topQueued.map((l) => (
              <li key={l.id}>
                <Score value={l.score} />
                <div>
                  <Link to={`/jobs/listings/${l.id}`} className="adm-row-title">
                    {l.title}
                  </Link>
                  <div className="adm-row-sub">{meta([l.company, PORTAL_LABEL[l.portal], l.workMode, l.location])}</div>
                </div>
              </li>
            ))}
          </ul>
        )}
      </Panel>

      <Panel title="Recent activity" actions={<Link to="/jobs/activity">All</Link>}>
        {o.recentTasks.length === 0 ? (
          <p className="adm-muted">No runs yet.</p>
        ) : (
          <ul className="job-mini-list">
            {o.recentTasks.map((t) => (
              <li key={t.id}>
                <span className={`job-dot job-dot--${t.status}`} aria-label={t.status} />
                <div>
                  <span>
                    {t.kind === 'search' ? `Search “${t.searchName}”` : t.kind === 'login' ? 'Sign-in' : 'Session check'} on{' '}
                    {t.portal ? PORTAL_LABEL[t.portal] : '—'}
                  </span>
                  <div className="adm-row-sub">
                    {meta([
                      t.status,
                      t.kind === 'search' && t.status === 'done' && `${t.result.new ?? 0} new, ${t.result.queued ?? 0} queued`,
                      t.error,
                      formatDate(t.finishedAt ?? t.createdAt),
                    ])}
                  </div>
                </div>
              </li>
            ))}
          </ul>
        )}
      </Panel>
    </>
  );
}

function Stat({ label, value, to, hot }: { label: string; value: number; to: string; hot?: boolean }) {
  return (
    <Link to={to} className={`adm-stat${hot ? ' is-hot' : ''}`}>
      <span className="adm-stat-value">{value.toLocaleString()}</span>
      <span className="adm-stat-label">{label}</span>
    </Link>
  );
}
