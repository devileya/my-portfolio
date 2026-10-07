import { useState, type FormEvent } from 'react';
import { adminApi } from '../lib/api';
import { SKILL_LEVELS, type Skill, type SkillLevel } from '../lib/types';
import { useReorder } from './reorder';
import { useSession } from './session';
import { ConfirmButton, ErrorNote, Loading, MoveButtons, PageHeader, Panel, useLoad } from './ui';

export default function SkillsPage() {
  const { errorText, notify } = useSession();
  const { data, setData, error, reload } = useLoad(adminApi.skills.list);
  const { moveItem, saving } = useReorder(data, setData, adminApi.skills.reorder);
  const [draft, setDraft] = useState({ name: '', category: '', level: 'Advanced' as SkillLevel });
  const [adding, setAdding] = useState(false);
  const categories = Array.from(new Set((data ?? []).map((s) => s.category).filter(Boolean)));

  async function add(e: FormEvent) {
    e.preventDefault();
    setAdding(true);
    try {
      const created = await adminApi.skills.create({ ...draft, sortOrder: 0 });
      setData([...(data ?? []), created]);
      setDraft((d) => ({ ...d, name: '' }));
      notify('Skill added');
    } catch (err) {
      notify(errorText(err), 'error');
    } finally {
      setAdding(false);
    }
  }

  function replace(s: Skill) {
    setData((data ?? []).map((x) => (x.id === s.id ? s : x)));
  }

  return (
    <>
      <PageHeader title="Skills" hint="Grouped by category on the site, in this order. Levels come from your CV’s scale." />
      {error && <ErrorNote text={error} onRetry={reload} />}
      {!data && !error && <Loading />}

      <Panel title="Add a skill">
        <form className="adm-skill-row adm-skill-row--add" onSubmit={add}>
          <input
            aria-label="Skill name"
            placeholder="Name, e.g. LangGraph"
            value={draft.name}
            onChange={(e) => setDraft({ ...draft, name: e.target.value })}
          />
          <input
            aria-label="Category"
            placeholder="Category"
            list="skill-categories"
            value={draft.category}
            onChange={(e) => setDraft({ ...draft, category: e.target.value })}
          />
          <select aria-label="Level" value={draft.level} onChange={(e) => setDraft({ ...draft, level: e.target.value as SkillLevel })}>
            {SKILL_LEVELS.map((l) => (
              <option key={l}>{l}</option>
            ))}
          </select>
          <button type="submit" className="adm-btn adm-btn--primary" disabled={adding || !draft.name.trim()}>
            {adding ? 'Adding…' : 'Add skill'}
          </button>
        </form>
        <datalist id="skill-categories">
          {categories.map((c) => (
            <option key={c} value={c} />
          ))}
        </datalist>
      </Panel>

      {data && data.length === 0 && <p className="adm-muted">No skills yet. Add one above.</p>}
      {data && data.length > 0 && (
        <Panel title={`${data.length} skills`}>
          <ul className="adm-skill-list">
            {data.map((s, i) => (
              <SkillRow
                key={s.id}
                skill={s}
                index={i}
                count={data.length}
                reordering={saving}
                onMove={(to) => moveItem(i, to)}
                onSaved={replace}
                onDeleted={() => setData((data ?? []).filter((x) => x.id !== s.id))}
              />
            ))}
          </ul>
        </Panel>
      )}
    </>
  );
}

function SkillRow({
  skill,
  index,
  count,
  reordering,
  onMove,
  onSaved,
  onDeleted,
}: {
  skill: Skill;
  index: number;
  count: number;
  reordering: boolean;
  onMove: (to: number) => void;
  onSaved: (s: Skill) => void;
  onDeleted: () => void;
}) {
  const { errorText, notify } = useSession();
  const [s, setS] = useState(skill);
  const [busy, setBusy] = useState(false);
  const dirty = s.name !== skill.name || s.category !== skill.category || s.level !== skill.level;

  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      const saved = await adminApi.skills.update(s);
      setS(saved);
      onSaved(saved);
      notify('Skill saved');
    } catch (err) {
      notify(errorText(err), 'error');
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    setBusy(true);
    try {
      await adminApi.skills.remove(skill.id);
      onDeleted();
      notify('Skill deleted');
    } catch (err) {
      notify(errorText(err), 'error');
      setBusy(false);
    }
  }

  return (
    <li>
      <form className="adm-skill-row" onSubmit={save}>
        <MoveButtons index={index} count={count} onMove={onMove} label={skill.name} disabled={reordering} />
        <input aria-label="Skill name" value={s.name} onChange={(e) => setS({ ...s, name: e.target.value })} />
        <input aria-label="Category" list="skill-categories" value={s.category} onChange={(e) => setS({ ...s, category: e.target.value })} />
        <select aria-label="Level" value={s.level} onChange={(e) => setS({ ...s, level: e.target.value as SkillLevel })}>
          {SKILL_LEVELS.map((l) => (
            <option key={l}>{l}</option>
          ))}
        </select>
        <span className="adm-row-actions">
          <button type="submit" className="adm-btn adm-btn--ghost" disabled={busy || !dirty || !s.name.trim()}>
            Save
          </button>
          <ConfirmButton label="Delete" confirmLabel="Delete skill" busy={busy} onConfirm={remove} />
        </span>
      </form>
    </li>
  );
}
