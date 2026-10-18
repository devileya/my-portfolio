import { useState, type FormEvent } from 'react';
import { jobsApi } from '../../lib/api';
import type { SavedAnswer } from '../../lib/jobs-types';
import { useSession } from '../session';
import { Badge, ConfirmButton, ErrorNote, Loading, PageHeader, Panel, useLoad } from '../ui';

const STARTERS = [
  'How many years of work experience do you have?',
  'What is your expected salary?',
  'What is your notice period?',
  'Are you willing to relocate?',
  'Do you require visa sponsorship?',
  'Are you comfortable working on-site?',
];

export default function AnswersPage() {
  const { errorText, notify } = useSession();
  const { data, setData, error, reload } = useLoad(jobsApi.answers);
  const [draft, setDraft] = useState({ question: '', answer: '' });
  const [busy, setBusy] = useState(false);
  const suggestions = (data ?? []).filter((a) => !a.approved);
  const approved = (data ?? []).filter((a) => a.approved);
  const missingStarters = STARTERS.filter((q) => !(data ?? []).some((a) => a.question.toLowerCase() === q.toLowerCase()));

  async function add(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      const saved = await jobsApi.createAnswer(draft);
      setData([saved, ...(data ?? []).filter((a) => a.id !== saved.id)]);
      setDraft({ question: '', answer: '' });
      notify('Answer saved');
    } catch (err) {
      notify(errorText(err), 'error');
    } finally {
      setBusy(false);
    }
  }

  const replace = (a: SavedAnswer) => setData((data ?? []).map((x) => (x.id === a.id ? a : x)));
  const drop = (id: number) => setData((data ?? []).filter((x) => x.id !== id));

  return (
    <>
      <PageHeader title="Answer bank" hint="Screening questions and your answers. The worker uses these first and only asks the AI for the rest." />
      {error && <ErrorNote text={error} onRetry={reload} />}
      {!data && !error && <Loading />}

      <Panel title="Add an answer">
        <form className="job-answer-form" onSubmit={add}>
          <input
            aria-label="Question"
            placeholder="Question as forms ask it"
            list="answer-starters"
            value={draft.question}
            onChange={(e) => setDraft({ ...draft, question: e.target.value })}
          />
          <input aria-label="Answer" placeholder="Your answer" value={draft.answer} onChange={(e) => setDraft({ ...draft, answer: e.target.value })} />
          <button type="submit" className="adm-btn adm-btn--primary" disabled={busy || !draft.question.trim() || !draft.answer.trim()}>
            Add
          </button>
        </form>
        <datalist id="answer-starters">
          {missingStarters.map((q) => (
            <option key={q} value={q} />
          ))}
        </datalist>
        {missingStarters.length > 0 && <p className="adm-hint">Common ones you haven’t answered: {missingStarters.join(' · ')}</p>}
      </Panel>

      {suggestions.length > 0 && (
        <Panel title={`AI suggestions to confirm (${suggestions.length})`}>
          <p className="adm-hint">The AI inferred these while filling forms. Confirm or correct them and they’ll be reused as-is.</p>
          <ul className="job-answer-list">
            {suggestions.map((a) => (
              <AnswerRow key={a.id} a={a} onSaved={replace} onDeleted={() => drop(a.id)} />
            ))}
          </ul>
        </Panel>
      )}

      {data && (
        <Panel title={`Your answers (${approved.length})`}>
          {approved.length === 0 ? (
            <p className="adm-muted">None yet.</p>
          ) : (
            <ul className="job-answer-list">
              {approved.map((a) => (
                <AnswerRow key={a.id} a={a} onSaved={replace} onDeleted={() => drop(a.id)} />
              ))}
            </ul>
          )}
        </Panel>
      )}
    </>
  );
}

function AnswerRow({ a, onSaved, onDeleted }: { a: SavedAnswer; onSaved: (a: SavedAnswer) => void; onDeleted: () => void }) {
  const { errorText, notify } = useSession();
  const [v, setV] = useState(a);
  const [busy, setBusy] = useState(false);
  const dirty = v.question !== a.question || v.answer !== a.answer;

  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      const saved = await jobsApi.updateAnswer(v);
      setV(saved);
      onSaved(saved);
      notify(a.approved ? 'Answer saved' : 'Confirmed');
    } catch (err) {
      notify(errorText(err), 'error');
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    setBusy(true);
    try {
      await jobsApi.deleteAnswer(a.id);
      onDeleted();
    } catch (err) {
      notify(errorText(err), 'error');
      setBusy(false);
    }
  }

  return (
    <li>
      <form className="job-answer-form" onSubmit={save}>
        <input aria-label="Question" value={v.question} onChange={(e) => setV({ ...v, question: e.target.value })} />
        <input aria-label="Answer" value={v.answer} onChange={(e) => setV({ ...v, answer: e.target.value })} />
        <span className="adm-row-actions">
          {a.usedCount > 0 && <Badge tone="dim">used {a.usedCount}×</Badge>}
          <button type="submit" className={`adm-btn ${a.approved ? 'adm-btn--ghost' : 'adm-btn--primary'}`} disabled={busy || (a.approved && !dirty) || !v.answer.trim()}>
            {a.approved ? 'Save' : 'Confirm'}
          </button>
          <ConfirmButton label="Delete" confirmLabel="Delete" busy={busy} onConfirm={remove} />
        </span>
      </form>
    </li>
  );
}
