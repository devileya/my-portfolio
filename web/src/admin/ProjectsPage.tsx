import { useState, type FormEvent } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { adminApi } from '../lib/api';
import { CATEGORY_LABEL, type Category, type Project } from '../lib/types';
import { useReorder } from './reorder';
import { useSession } from './session';
import {
  Badge,
  ConfirmButton,
  ErrorNote,
  Field,
  ImageUpload,
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

export default function ProjectsPage() {
  const { errorText, notify } = useSession();
  const { data, setData, error, reload } = useLoad(adminApi.projects.list);
  const { moveItem, saving } = useReorder(data, setData, adminApi.projects.reorder);
  const [deleting, setDeleting] = useState<number | null>(null);

  async function remove(p: Project) {
    setDeleting(p.id);
    try {
      await adminApi.projects.remove(p.id);
      setData((data ?? []).filter((x) => x.id !== p.id));
      notify('Project deleted');
    } catch (err) {
      notify(errorText(err), 'error');
    } finally {
      setDeleting(null);
    }
  }

  return (
    <>
      <PageHeader
        title="Projects"
        hint="Order here is the order on the site. Featured projects get a “featured” badge."
        actions={
          <Link to="/projects/new" className="adm-btn adm-btn--primary">
            Add project
          </Link>
        }
      />
      {error && <ErrorNote text={error} onRetry={reload} />}
      {!data && !error && <Loading />}
      {data && data.length === 0 && (
        <Panel>
          <p className="adm-muted">
            No projects yet. <Link to="/projects/new">Add your first project</Link> and it appears on the site once published.
          </p>
        </Panel>
      )}
      {data && data.length > 0 && (
        <ul className="adm-rows-list">
          {data.map((p, i) => (
            <li key={p.id} className={p.published ? '' : 'is-draft'}>
              <MoveButtons index={i} count={data.length} onMove={(to) => moveItem(i, to)} label={p.title} disabled={saving} />
              <div className="adm-thumb">{p.imageUrl ? <img src={p.imageUrl} alt="" loading="lazy" /> : null}</div>
              <div className="adm-row-main">
                <Link to={`/projects/${p.id}`} className="adm-row-title">
                  {p.title}
                </Link>
                <div className="adm-row-sub">
                  <Badge>{CATEGORY_LABEL[p.category] ?? p.category}</Badge>
                  {p.featured && <Badge tone="hot">Featured</Badge>}
                  {!p.published && <Badge tone="dim">Draft</Badge>}
                  {p.platforms && <span className="adm-muted">{p.platforms}</span>}
                </div>
              </div>
              <div className="adm-row-actions">
                <Link to={`/projects/${p.id}`} className="adm-btn adm-btn--ghost">
                  Edit
                </Link>
                <ConfirmButton label="Delete" confirmLabel="Delete project" busy={deleting === p.id} onConfirm={() => remove(p)} />
              </div>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}

const BLANK: Omit<Project, 'id'> = {
  title: '',
  category: 'web',
  platforms: '',
  summary: '',
  description: '',
  tech: [],
  imageKey: '',
  imageUrl: '',
  liveUrl: '',
  altUrl: '',
  repoUrl: '',
  featured: false,
  published: true,
  sortOrder: 0,
};

export function ProjectEditor() {
  const { id } = useParams();
  const isNew = !id;
  const { data, error, reload } = useLoad(() =>
    isNew
      ? Promise.resolve({ ...BLANK, id: 0 } as Project)
      : adminApi.projects.list().then((all) => {
          const found = all.find((p) => p.id === Number(id));
          if (!found) throw new Error('That project no longer exists.');
          return found;
        }),
  );

  if (error) return <ErrorNote text={error} onRetry={reload} />;
  if (!data) return <Loading />;
  return <ProjectForm key={data.id} initial={data} />;
}

function ProjectForm({ initial }: { initial: Project }) {
  const { errorText, notify } = useSession();
  const navigate = useNavigate();
  const [p, setP] = useState(initial);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const isNew = initial.id === 0;
  const set = <K extends keyof Project>(key: K, v: Project[K]) => setP((cur) => ({ ...cur, [key]: v }));

  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      if (isNew) {
        const { id: _omit, ...rest } = p;
        void _omit;
        await adminApi.projects.create(rest);
        notify('Project added');
      } else {
        await adminApi.projects.update(p);
        notify('Project saved');
      }
      navigate('/projects');
    } catch (err) {
      setError(errorText(err));
      setBusy(false);
    }
  }

  return (
    <form onSubmit={save}>
      <PageHeader
        title={isNew ? 'Add project' : `Edit ${initial.title}`}
        actions={
          <>
            <Link to="/projects" className="adm-btn adm-btn--ghost">
              Cancel
            </Link>
            <button type="submit" className="adm-btn adm-btn--primary" disabled={busy || !p.title.trim()}>
              {busy ? 'Saving…' : isNew ? 'Add project' : 'Save project'}
            </button>
          </>
        }
      />
      {error && <ErrorNote text={error} />}

      <Panel title="Basics">
        <div className="adm-grid">
          <TextField label="Title" value={p.title} onChange={(v) => set('title', v)} required />
          <Field label="Category">
            {(fid) => (
              <select id={fid} value={p.category} onChange={(e) => set('category', e.target.value as Category)}>
                {(Object.keys(CATEGORY_LABEL) as Category[]).map((c) => (
                  <option key={c} value={c}>
                    {CATEGORY_LABEL[c]}
                  </option>
                ))}
              </select>
            )}
          </Field>
          <TextField label="Platforms" value={p.platforms} onChange={(v) => set('platforms', v)} hint="e.g. Android · iOS" />
          <div className="adm-field adm-field--toggle">
            <Toggle label="Published" checked={p.published} onChange={(v) => set('published', v)} />
            <Toggle label="Featured" checked={p.featured} onChange={(v) => set('featured', v)} />
          </div>
          <TextArea label="Summary" rows={2} value={p.summary} onChange={(v) => set('summary', v)} hint="One sentence shown in the list." />
          <TextArea
            label="Description"
            rows={6}
            value={p.description}
            onChange={(v) => set('description', v)}
            hint="Shown when a visitor opens the project. Leave a blank line between paragraphs."
          />
          <TagsField label="Tech" value={p.tech} onChange={(v) => set('tech', v)} />
          <ImageUpload label="Image" url={p.imageUrl} onChange={(key, url) => setP((cur) => ({ ...cur, imageKey: key, imageUrl: url }))} />
        </div>
      </Panel>

      <Panel title="Links">
        <div className="adm-grid">
          <TextField label="Live URL" value={p.liveUrl} onChange={(v) => set('liveUrl', v)} placeholder="https://…" />
          <TextField label="Second URL" value={p.altUrl} onChange={(v) => set('altUrl', v)} hint="e.g. the App Store link" placeholder="https://…" />
          <TextField label="Source code URL" value={p.repoUrl} onChange={(v) => set('repoUrl', v)} placeholder="https://github.com/…" />
        </div>
      </Panel>
    </form>
  );
}
