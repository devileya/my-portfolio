import { useState, type FormEvent } from 'react';
import { adminApi } from '../lib/api';
import type { Fact, Profile, Social } from '../lib/types';
import { useSession } from './session';
import {
  ErrorNote,
  ImageUpload,
  ListEditor,
  Loading,
  PageHeader,
  Panel,
  RowsEditor,
  TextArea,
  TextField,
  Toggle,
  useLoad,
} from './ui';

export default function ProfilePage() {
  const { data, setData, error, reload } = useLoad(adminApi.profile);

  if (error && !data) return <ErrorNote text={error} onRetry={reload} />;
  if (!data) return <Loading />;
  return <ProfileForm initial={data} onSaved={setData} />;
}

function ProfileForm({ initial, onSaved }: { initial: Profile; onSaved: (p: Profile) => void }) {
  const { errorText, notify } = useSession();
  const [p, setP] = useState(initial);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const dirty = JSON.stringify(p) !== JSON.stringify(initial);

  const set = <K extends keyof Profile>(key: K, v: Profile[K]) => setP((cur) => ({ ...cur, [key]: v }));

  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const saved = await adminApi.saveProfile(p);
      setP(saved);
      onSaved(saved);
      notify('Profile saved');
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={save}>
      <PageHeader
        title="Profile"
        hint="Everything in the hero, the model card and the contact section."
        actions={
          <button type="submit" className="adm-btn adm-btn--primary" disabled={busy || !dirty}>
            {busy ? 'Saving…' : 'Save profile'}
          </button>
        }
      />
      {error && <ErrorNote text={error} />}

      <Panel title="Identity">
        <div className="adm-grid">
          <TextField label="Name" value={p.name} onChange={(v) => set('name', v)} required />
          <TextField label="Role" value={p.role} onChange={(v) => set('role', v)} />
          <TextField label="Email" type="email" value={p.email} onChange={(v) => set('email', v)} />
          <TextField label="Location" value={p.location} onChange={(v) => set('location', v)} hint="Leave empty to hide." />
          <TextField
            label="Years of experience"
            type="number"
            value={String(p.yearsExperience)}
            onChange={(v) => set('yearsExperience', Math.max(0, Number.parseInt(v, 10) || 0))}
          />
          <div className="adm-field adm-field--toggle">
            <Toggle label="Open to new work" checked={p.available} onChange={(v) => set('available', v)} />
          </div>
          <ImageUpload
            label="Photo"
            url={p.avatarUrl}
            onChange={(key, url) => setP((cur) => ({ ...cur, avatarKey: key, avatarUrl: url }))}
          />
        </div>
      </Panel>

      <Panel title="Hero">
        <div className="adm-grid">
          <TextField
            label="Prompt"
            wide
            value={p.prompt}
            onChange={(v) => set('prompt', v)}
            hint="The question shown above the generated headline."
          />
          <div className="adm-field adm-field--wide">
            <ListEditor
              label="Headlines"
              hint="The first one is the default answer at temperature 0; the others are sampled as temperature rises."
              items={p.headlines}
              onChange={(v) => set('headlines', v)}
              addLabel="Add headline"
              multiline
            />
          </div>
          <TextArea
            label="Summary"
            rows={3}
            value={p.summary}
            onChange={(v) => set('summary', v)}
            hint="One or two sentences. Also used as the search and link-preview description."
          />
        </div>
      </Panel>

      <Panel title="About">
        <div className="adm-grid">
          <TextArea label="Bio" rows={9} value={p.bio} onChange={(v) => set('bio', v)} hint="Leave a blank line between paragraphs." />
          <div className="adm-field adm-field--wide">
            <ListEditor label="Focus areas" items={p.focusAreas} onChange={(v) => set('focusAreas', v)} addLabel="Add focus area" />
          </div>
          <div className="adm-field adm-field--wide">
            <RowsEditor<Fact & Record<string, string | undefined>>
              label="Model card facts"
              rows={p.facts as (Fact & Record<string, string | undefined>)[]}
              columns={[
                { key: 'label', label: 'Label', placeholder: 'e.g. training data' },
                { key: 'value', label: 'Value' },
                { key: 'url', label: 'Link', placeholder: 'https://… (optional)' },
              ]}
              onChange={(v) => set('facts', v.map(({ label, value, url }) => (url ? { label, value, url } : { label, value })))}
              blank={{ label: '', value: '', url: '' }}
              addLabel="Add fact"
            />
          </div>
          <div className="adm-field adm-field--wide">
            <RowsEditor<Social & Record<string, string>>
              label="Social links"
              rows={p.socials as (Social & Record<string, string>)[]}
              columns={[
                { key: 'label', label: 'Label', placeholder: 'e.g. GitHub' },
                { key: 'url', label: 'URL', placeholder: 'https://…' },
              ]}
              onChange={(v) => set('socials', v.map(({ label, url }) => ({ label, url })))}
              blank={{ label: '', url: '' }}
              addLabel="Add link"
            />
          </div>
        </div>
      </Panel>

      <div className="adm-savebar">
        <span className="adm-muted">{dirty ? 'You have unsaved changes.' : 'All changes saved.'}</span>
        <button type="submit" className="adm-btn adm-btn--primary" disabled={busy || !dirty}>
          {busy ? 'Saving…' : 'Save profile'}
        </button>
      </div>
    </form>
  );
}
