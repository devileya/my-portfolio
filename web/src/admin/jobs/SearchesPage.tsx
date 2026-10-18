import { useState, type FormEvent } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { jobsApi } from '../../lib/api';
import { JOB_TYPES, PORTALS, PORTAL_LABEL, SENIORITY, WORK_MODES, label, type JobSearch, type Portal } from '../../lib/jobs-types';
import { useSession } from '../session';
import { Badge, ConfirmButton, ErrorNote, Field, Loading, PageHeader, Panel, TagsField, TextArea, TextField, Toggle, useLoad } from '../ui';
import { Chips, ago, meta } from './common';

const SCHEDULES = [
  { cron: '0 9,13,17 * * *', text: 'Three times a day (09:00, 13:00, 17:00)' },
  { cron: '0 8-20/3 * * *', text: 'Every 3 hours, 08:00–20:00' },
  { cron: '0 9 * * *', text: 'Daily at 09:00' },
  { cron: '0 9,15 * * 1-5', text: 'Weekdays at 09:00 and 15:00' },
  { cron: '0 */1 * * *', text: 'Every hour' },
];

export default function SearchesPage() {
  const { errorText, notify } = useSession();
  const { data, setData, error, reload } = useLoad(jobsApi.searches);
  const [busy, setBusy] = useState<number | null>(null);

  async function run(s: JobSearch) {
    setBusy(s.id);
    try {
      const r = await jobsApi.runSearch(s.id);
      notify(r.queued > 0 ? `Queued on ${r.queued} portal${r.queued === 1 ? '' : 's'}` : 'Already queued');
    } catch (err) {
      notify(errorText(err), 'error');
    } finally {
      setBusy(null);
    }
  }

  async function toggle(s: JobSearch) {
    setBusy(s.id);
    try {
      const saved = await jobsApi.updateSearch({ ...s, enabled: !s.enabled });
      setData((data ?? []).map((x) => (x.id === s.id ? saved : x)));
    } catch (err) {
      notify(errorText(err), 'error');
    } finally {
      setBusy(null);
    }
  }

  return (
    <>
      <PageHeader
        title="Saved searches"
        hint="Each search runs on its own schedule across the portals you pick."
        actions={
          <Link to="/jobs/searches/new" className="adm-btn adm-btn--primary">
            New search
          </Link>
        }
      />
      {error && <ErrorNote text={error} onRetry={reload} />}
      {!data && !error && <Loading />}
      {data && data.length === 0 && (
        <Panel>
          <p className="adm-muted">
            No searches yet. <Link to="/jobs/searches/new">Create one</Link>: keywords, locations, remote/hybrid/on-site and a schedule.
          </p>
        </Panel>
      )}
      {data && data.length > 0 && (
        <ul className="adm-rows-list">
          {data.map((s) => (
            <li key={s.id} className={s.enabled ? '' : 'is-draft'}>
              <div className="adm-row-main">
                <Link to={`/jobs/searches/${s.id}`} className="adm-row-title">
                  {s.name}
                </Link>
                <div className="adm-row-sub">
                  {!s.enabled && <Badge tone="dim">Off</Badge>}
                  {s.autoApply && <Badge tone="hot">Auto ≥ {s.autoMinScore}</Badge>}
                  <span>{s.keywords.join(', ')}</span>
                  <span>{meta([s.workModes.map(label).join('/'), s.locations.join(', '), s.portals.map((p) => PORTAL_LABEL[p]).join(', ')])}</span>
                </div>
                <div className="adm-row-sub">
                  <span className="adm-muted">
                    Last run {ago(s.lastRunAt)} · next {s.enabled ? ago(s.nextRunAt) : '—'}
                  </span>
                </div>
              </div>
              <div className="adm-row-actions">
                <button type="button" className="adm-btn adm-btn--ghost" disabled={busy === s.id} onClick={() => toggle(s)}>
                  {s.enabled ? 'Turn off' : 'Turn on'}
                </button>
                <button type="button" className="adm-btn adm-btn--ghost" disabled={busy === s.id || !s.enabled} onClick={() => run(s)}>
                  Run now
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}

const blank: Omit<JobSearch, 'id'> = {
  name: '',
  enabled: true,
  portals: [...PORTALS],
  keywords: [],
  excludeKeywords: [],
  excludeCompanies: [],
  locations: ['Indonesia'],
  workModes: ['remote', 'hybrid'],
  jobTypes: ['full_time', 'contract'],
  seniority: [],
  minSalary: 0,
  postedWithinDays: 7,
  easyApplyOnly: true,
  schedule: SCHEDULES[0].cron,
  maxResults: 25,
  minScore: 65,
  autoApply: false,
  autoMinScore: 85,
  dailyCap: 15,
  notes: '',
  lastRunAt: null,
  nextRunAt: null,
  createdAt: '',
  updatedAt: '',
};

export function SearchEditor() {
  const { id } = useParams();
  const isNew = !id;
  const { data, error, reload } = useLoad(() =>
    isNew ? Promise.resolve(blank as JobSearch) : jobsApi.searches().then((all) => {
      const s = all.find((x) => x.id === Number(id));
      if (!s) throw new Error('That search no longer exists.');
      return s;
    }),
  );
  if (error && !data) return <ErrorNote text={error} onRetry={reload} />;
  if (!data) return <Loading />;
  return <SearchForm initial={data} isNew={isNew} />;
}

function SearchForm({ initial, isNew }: { initial: JobSearch; isNew: boolean }) {
  const { errorText, notify } = useSession();
  const navigate = useNavigate();
  const [s, setS] = useState(initial);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const preset = SCHEDULES.find((x) => x.cron === s.schedule);
  const [custom, setCustom] = useState(!preset);
  const set = <K extends keyof JobSearch>(k: K, v: JobSearch[K]) => setS((cur) => ({ ...cur, [k]: v }));
  const num = (v: string) => Math.max(0, Number.parseInt(v || '0', 10) || 0);

  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      if (isNew) {
        const created = await jobsApi.createSearch(s);
        notify('Search created');
        navigate(`/jobs/searches/${created.id}`, { replace: true });
      } else {
        setS(await jobsApi.updateSearch(s));
        notify('Search saved');
      }
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    setBusy(true);
    try {
      await jobsApi.deleteSearch(s.id);
      notify('Search deleted');
      navigate('/jobs/searches');
    } catch (err) {
      notify(errorText(err), 'error');
      setBusy(false);
    }
  }

  return (
    <form onSubmit={save}>
      <p className="adm-muted">
        <Link to="/jobs/searches">← Saved searches</Link>
      </p>
      <PageHeader
        title={isNew ? 'New search' : s.name}
        actions={
          <>
            {!isNew && <ConfirmButton label="Delete" confirmLabel="Delete search" busy={busy} onConfirm={remove} />}
            <button type="submit" className="adm-btn adm-btn--primary" disabled={busy}>
              {busy ? 'Saving…' : isNew ? 'Create search' : 'Save'}
            </button>
          </>
        }
      />
      {error && <ErrorNote text={error} />}

      <Panel title="What to look for">
        <div className="adm-grid">
          <TextField
            label="Name"
            value={s.name}
            onChange={(v) => set('name', v)}
            placeholder="e.g. Senior Go / AI roles"
            hint="A label for you: shown in this list, Telegram alerts and the activity log. Keywords decide what is searched."
            required
          />
          <div className="adm-field adm-field--toggle">
            <Toggle label="Enabled" checked={s.enabled} onChange={(v) => set('enabled', v)} />
          </div>
          <TagsField label="Keywords" value={s.keywords} onChange={(v) => set('keywords', v)} hint="Each keyword is its own query, e.g. Golang developer, Backend engineer, AI engineer. Forms asking for your preferred role, work setup, location or job type are answered from this search." />
          <TagsField label="Locations" value={s.locations} onChange={(v) => set('locations', v)} hint="Cities or countries, e.g. Jakarta, Indonesia, Singapore. Remote jobs match any location." />
          <div className="adm-field--wide">
            <Chips legend="Work mode" options={WORK_MODES} value={s.workModes} onChange={(v) => set('workModes', v)} hint="None selected = any." />
          </div>
          <div className="adm-field--wide">
            <Chips legend="Job type" options={JOB_TYPES} value={s.jobTypes} onChange={(v) => set('jobTypes', v)} />
          </div>
          <div className="adm-field--wide">
            <Chips legend="Seniority" options={SENIORITY} value={s.seniority} onChange={(v) => set('seniority', v)} hint="Used as a portal filter where supported, and by the scorer." />
          </div>
          <div className="adm-field--wide">
            <Chips
              legend="Portals"
              options={PORTALS}
              value={s.portals}
              onChange={(v) => set('portals', v as Portal[])}
              format={(p) => PORTAL_LABEL[p as Portal]}
            />
          </div>
        </div>
      </Panel>

      <Panel title="Filters">
        <div className="adm-grid">
          <TagsField label="Exclude title keywords" value={s.excludeKeywords} onChange={(v) => set('excludeKeywords', v)} hint="e.g. PHP, Intern, Sales." />
          <TagsField label="Exclude companies" value={s.excludeCompanies} onChange={(v) => set('excludeCompanies', v)} />
          <TextField label="Posted within (days)" type="number" value={String(s.postedWithinDays)} onChange={(v) => set('postedWithinDays', num(v))} hint="0 = any time." />
          <TextField label="Minimum monthly salary (IDR)" type="number" value={String(s.minSalary)} onChange={(v) => set('minSalary', num(v))} hint="In IDR. Told to the scorer, and given as your expected salary when a posting shows no range. 0 = none." />
          <div className="adm-field adm-field--toggle adm-field--wide">
            <Toggle label="Only jobs you can apply to on the portal (Easy Apply / Lamar)" checked={s.easyApplyOnly} onChange={(v) => set('easyApplyOnly', v)} />
          </div>
          <TextArea label="Notes for the scorer" value={s.notes} onChange={(v) => set('notes', v)} rows={3} hint="Preferences in your words, e.g. “prefer product companies, avoid outsourcing agencies, open to relocating to Singapore”." />
        </div>
      </Panel>

      <Panel title="Schedule & automation">
        <div className="adm-grid">
          <Field label="Runs" hint={custom ? 'Cron: minute hour day month weekday, in the Job Hunter time zone.' : undefined}>
            {(fid) =>
              custom ? (
                <input id={fid} value={s.schedule} onChange={(e) => set('schedule', e.target.value)} placeholder="0 9 * * 1-5" />
              ) : (
                <select
                  id={fid}
                  value={s.schedule}
                  onChange={(e) => (e.target.value === 'custom' ? setCustom(true) : set('schedule', e.target.value))}
                >
                  {SCHEDULES.map((x) => (
                    <option key={x.cron} value={x.cron}>
                      {x.text}
                    </option>
                  ))}
                  <option value="custom">Custom (cron)…</option>
                </select>
              )
            }
          </Field>
          <TextField label="Results per run, per portal" type="number" value={String(s.maxResults)} onChange={(v) => set('maxResults', num(v))} />
          <TextField label="Review-queue score" type="number" value={String(s.minScore)} onChange={(v) => set('minScore', num(v))} hint="Matches scoring at least this enter the queue; lower ones are filtered." />
          <TextField label="Daily application cap for this search" type="number" value={String(s.dailyCap)} onChange={(v) => set('dailyCap', num(v))} hint="Portal caps in Settings also apply." />
          <div className="adm-field adm-field--toggle">
            <Toggle label="Auto-apply to strong matches" checked={s.autoApply} onChange={(v) => set('autoApply', v)} />
          </div>
          <TextField
            label="Auto-apply score"
            type="number"
            value={String(s.autoMinScore)}
            onChange={(v) => set('autoMinScore', num(v))}
            hint={s.autoApply ? 'Matches at or above this are approved without review.' : 'Turn on auto-apply to use this.'}
          />
        </div>
      </Panel>

      <div className="adm-savebar">
        <span className="adm-muted">{isNew ? 'The first run happens at the next scheduled time, or press Run now.' : `Next run ${ago(s.nextRunAt)}.`}</span>
        <button type="submit" className="adm-btn adm-btn--primary" disabled={busy}>
          {busy ? 'Saving…' : isNew ? 'Create search' : 'Save'}
        </button>
      </div>
    </form>
  );
}
