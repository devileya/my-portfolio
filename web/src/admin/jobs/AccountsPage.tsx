import { useState } from 'react';
import { jobsApi } from '../../lib/api';
import { PORTAL_LABEL, type JobAccount } from '../../lib/jobs-types';
import { useSession } from '../session';
import { ConfirmButton, ErrorNote, Loading, PageHeader, Panel, Toggle, useLoad } from '../ui';
import { AccountBadge, ago, usePoll } from './common';

const NOTES: Record<string, string> = {
  linkedin: 'Only Easy Apply jobs are applied to here. Keep the daily cap modest: LinkedIn restricts accounts that apply in bulk.',
  indeed: 'Indeed is behind Cloudflare. If it shows a security check, sign in once in the worker’s Chrome window and solve it yourself.',
  glints: 'Glints applies with the CV on your Glints profile or the one uploaded here.',
  kalibrr: 'Kalibrr may ask for its own assessment on some jobs; those stop for your input.',
};

export default function AccountsPage() {
  const { data, setData, error, reload } = useLoad(jobsApi.accounts);
  usePoll(() => jobsApi.accounts().then(setData).catch(() => {}), 10000);

  return (
    <>
      <PageHeader
        title="Portal accounts"
        hint="Sessions are stored encrypted. Your passwords are never stored: you sign in yourself, or paste exported cookies."
      />
      {error && <ErrorNote text={error} onRetry={reload} />}
      {!data && !error && <Loading />}
      {data?.map((a) => <AccountCard key={a.portal} a={a} onChange={setData} />)}
    </>
  );
}

function AccountCard({ a, onChange }: { a: JobAccount; onChange: (list: JobAccount[]) => void }) {
  const { errorText, notify } = useSession();
  const [busy, setBusy] = useState(false);
  const [paste, setPaste] = useState<string | null>(null);

  async function run(fn: () => Promise<unknown>, done: string) {
    setBusy(true);
    try {
      const r = await fn();
      if (Array.isArray(r)) onChange(r as JobAccount[]);
      notify(done);
      return true;
    } catch (err) {
      notify(err instanceof SyntaxError ? 'That isn’t valid JSON.' : errorText(err), 'error');
      return false;
    } finally {
      setBusy(false);
    }
  }

  return (
    <Panel
      title={PORTAL_LABEL[a.portal]}
      actions={<Toggle label="Use this portal" checked={a.enabled} onChange={(v) => run(() => jobsApi.setAccountEnabled(a.portal, v), v ? 'Turned on' : 'Turned off')} />}
    >
      <div className="job-acct">
        <div>
          <p className="adm-row-sub">
            <AccountBadge status={a.status} enabled={a.enabled} />
            {a.cookiesCount > 0 && <span>{a.cookiesCount} cookies stored</span>}
            <span className="adm-muted">checked {ago(a.checkedAt)}</span>
          </p>
          {a.statusNote && <p className="job-reason">{a.statusNote}</p>}
          <p className="adm-hint">{NOTES[a.portal]}</p>
        </div>
        <div className="adm-row-actions job-acct-actions">
          <button
            type="button"
            className="adm-btn adm-btn--primary"
            disabled={busy}
            onClick={() => run(() => jobsApi.login(a.portal), 'Chrome is opening on the worker machine: sign in there within 10 minutes')}
          >
            Sign in on worker
          </button>
          <button type="button" className="adm-btn adm-btn--ghost" disabled={busy} onClick={() => setPaste(paste === null ? '' : null)}>
            Paste cookies
          </button>
          <button
            type="button"
            className="adm-btn adm-btn--ghost"
            disabled={busy || a.cookiesCount === 0}
            onClick={() => run(() => jobsApi.check(a.portal), 'Checking the session; refresh in a minute')}
          >
            Check session
          </button>
          {a.cookiesCount > 0 && (
            <ConfirmButton
              label="Forget session"
              confirmLabel="Forget"
              busy={busy}
              onConfirm={() => run(() => jobsApi.clearCookies(a.portal), 'Session forgotten')}
            />
          )}
        </div>
      </div>
      {paste !== null && (
        <div className="job-paste">
          <label htmlFor={`ck-${a.portal}`}>Cookies JSON</label>
          <p className="adm-hint">
            Sign in to {PORTAL_LABEL[a.portal]} in your own browser, export its cookies with an extension such as Cookie-Editor
            (Export → JSON), and paste them here.
          </p>
          <textarea id={`ck-${a.portal}`} rows={6} value={paste} onChange={(e) => setPaste(e.target.value)} placeholder='[{"name": "…", "value": "…", "domain": "…"}]' />
          <button
            type="button"
            className="adm-btn adm-btn--primary"
            disabled={busy || !paste.trim()}
            onClick={async () => {
              if (await run(() => jobsApi.importCookies(a.portal, paste), 'Cookies saved; checking the session')) setPaste(null);
            }}
          >
            Save cookies
          </button>
        </div>
      )}
    </Panel>
  );
}
