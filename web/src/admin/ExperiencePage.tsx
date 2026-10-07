import { useState, type FormEvent } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { adminApi } from '../lib/api';
import type { Experience } from '../lib/types';
import { useReorder } from './reorder';
import { useSession } from './session';
import {
  Badge,
  ConfirmButton,
  ErrorNote,
  Field,
  Loading,
  MoveButtons,
  PageHeader,
  Panel,
  TagsField,
  TextArea,
  TextField,
  Toggle,
  useLoad,
} from './ui';

function period(e: Experience) {
  const end = e.periodEnd || (e.kind === 'work' ? 'present' : '');
  return [e.periodStart, end].filter(Boolean).join(' – ');
}

export default function ExperiencePage() {
  const { errorText, notify } = useSession();
  const { data, setData, error, reload } = useLoad(adminApi.experiences.list);
  const { moveItem, saving } = useReorder(data, setData, adminApi.experiences.reorder);
  const [deleting, setDeleting] = useState<number | null>(null);

  async function remove(e: Experience) {
    setDeleting(e.id);
    try {
      await adminApi.experiences.remove(e.id);
      setData((data ?? []).filter((x) => x.id !== e.id));
      notify('Entry deleted');
    } catch (err) {
      notify(errorText(err), 'error');
    } finally {
      setDeleting(null);
    }
  }

  return (
    <>
      <PageHeader
        title="Experience"
        hint="Roles and degrees, in the order the timeline shows them."
        actions={
          <Link to="/experience/new" className="adm-btn adm-btn--primary">
            Add entry
          </Link>
        }
      />
      {error && <ErrorNote text={error} onRetry={reload} />}
      {!data && !error && <Loading />}
      {data && data.length === 0 && (
        <Panel>
          <p className="adm-muted">
            The timeline is empty. <Link to="/experience/new">Add a role or degree</Link> to show it on the site.
          </p>
        </Panel>
      )}
      {data && data.length > 0 && (
        <ul className="adm-rows-list">
          {data.map((e, i) => (
            <li key={e.id} className={e.published ? '' : 'is-draft'}>
              <MoveButtons index={i} count={data.length} onMove={(to) => moveItem(i, to)} label={e.title} disabled={saving} />
              <div className="adm-row-main">
                <Link to={`/experience/${e.id}`} className="adm-row-title">
                  {e.title}
                </Link>
                <div className="adm-row-sub">
                  <Badge tone={e.kind === 'education' ? 'dim' : 'plain'}>{e.kind === 'education' ? 'Education' : 'Work'}</Badge>
                  {!e.published && <Badge tone="dim">Draft</Badge>}
                  <span>{e.organization}</span>
                  <span className="adm-muted mono">{period(e)}</span>
                </div>
              </div>
              <div className="adm-row-actions">
                <Link to={`/experience/${e.id}`} className="adm-btn adm-btn--ghost">
                  Edit
                </Link>
                <ConfirmButton label="Delete" confirmLabel="Delete entry" busy={deleting === e.id} onConfirm={() => remove(e)} />
              </div>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}

const BLANK: Experience = {
  id: 0,
  kind: 'work',
  title: '',
  organization: '',
  periodStart: '',
  periodEnd: '',
  summary: '',
  highlights: [],
  tech: [],
  published: true,
  sortOrder: 0,
};

export function ExperienceEditor() {
  const { id } = useParams();
  const { data, error, reload } = useLoad(() =>
    !id
      ? Promise.resolve(BLANK)
      : adminApi.experiences.list().then((all) => {
          const found = all.find((e) => e.id === Number(id));
          if (!found) throw new Error('That entry no longer exists.');
          return found;
        }),
  );
  if (error) return <ErrorNote text={error} onRetry={reload} />;
  if (!data) return <Loading />;
  return <ExperienceForm key={data.id} initial={data} />;
}

function ExperienceForm({ initial }: { initial: Experience }) {
  const { errorText, notify } = useSession();
  const navigate = useNavigate();
  const [e, setE] = useState(initial);
  const [highlights, setHighlights] = useState(initial.highlights.join('\n'));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const isNew = initial.id === 0;
  const set = <K extends keyof Experience>(key: K, v: Experience[K]) => setE((cur) => ({ ...cur, [key]: v }));

  async function save(ev: FormEvent) {
    ev.preventDefault();
    setBusy(true);
    setError('');
    const value: Experience = {
      ...e,
      highlights: highlights
        .split('\n')
        .map((s) => s.trim())
        .filter(Boolean),
    };
    try {
      if (isNew) {
        const { id: _omit, ...rest } = value;
        void _omit;
        await adminApi.experiences.create(rest);
        notify('Entry added');
      } else {
        await adminApi.experiences.update(value);
        notify('Entry saved');
      }
      navigate('/experience');
    } catch (err) {
      setError(errorText(err));
      setBusy(false);
    }
  }

  return (
    <form onSubmit={save}>
      <PageHeader
        title={isNew ? 'Add entry' : `Edit ${initial.title}`}
        actions={
          <>
            <Link to="/experience" className="adm-btn adm-btn--ghost">
              Cancel
            </Link>
            <button type="submit" className="adm-btn adm-btn--primary" disabled={busy || !e.title.trim()}>
              {busy ? 'Saving…' : isNew ? 'Add entry' : 'Save entry'}
            </button>
          </>
        }
      />
      {error && <ErrorNote text={error} />}
      <Panel>
        <div className="adm-grid">
          <Field label="Type">
            {(fid) => (
              <select id={fid} value={e.kind} onChange={(ev) => set('kind', ev.target.value as Experience['kind'])}>
                <option value="work">Work</option>
                <option value="education">Education</option>
              </select>
            )}
          </Field>
          <div className="adm-field adm-field--toggle">
            <Toggle label="Published" checked={e.published} onChange={(v) => set('published', v)} />
          </div>
          <TextField
            label={e.kind === 'education' ? 'Degree' : 'Role'}
            value={e.title}
            onChange={(v) => set('title', v)}
            required
          />
          <TextField
            label={e.kind === 'education' ? 'School' : 'Organization'}
            value={e.organization}
            onChange={(v) => set('organization', v)}
          />
          <TextField label="Start" value={e.periodStart} onChange={(v) => set('periodStart', v)} hint="YYYY-MM or YYYY" placeholder="2023-11" />
          <TextField
            label="End"
            value={e.periodEnd}
            onChange={(v) => set('periodEnd', v)}
            hint="YYYY-MM or YYYY; leave end empty for present"
            placeholder="2025-05"
          />
          <TextArea label="Summary" rows={3} value={e.summary} onChange={(v) => set('summary', v)} />
          <TextArea label="Highlights" rows={6} value={highlights} onChange={setHighlights} hint="One per line." />
          <TagsField label="Tech" value={e.tech} onChange={(v) => set('tech', v)} />
        </div>
      </Panel>
    </form>
  );
}
