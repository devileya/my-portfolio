import { useState, type FormEvent } from 'react';
import { Link, useParams } from 'react-router-dom';
import { jobsApi } from '../../lib/api';
import { PORTAL_LABEL, label, type Application, type Question } from '../../lib/jobs-types';
import { useSession } from '../session';
import { Badge, ErrorNote, Loading, PageHeader, Panel, formatDate, useLoad } from '../ui';
import { Score, StatusBadge, meta } from './common';

export default function ListingPage() {
  const { id } = useParams();
  const { errorText, notify } = useSession();
  const { data, setData, error, reload } = useLoad(() => jobsApi.listing(Number(id)));
  const [busy, setBusy] = useState(false);

  async function act(fn: () => Promise<unknown>, done: string) {
    setBusy(true);
    try {
      await fn();
      notify(done);
      reload();
    } catch (err) {
      notify(errorText(err), 'error');
    } finally {
      setBusy(false);
    }
  }

  if (error && !data) return <ErrorNote text={error} onRetry={reload} />;
  if (!data) return <Loading />;
  const { listing: l, applications } = data;
  const latest = applications[0];
  const d = l.scoreDetails ?? {};

  return (
    <>
      <p className="adm-muted">
        <Link to="/jobs/queue">← Review queue</Link>
      </p>
      <PageHeader
        title={l.title}
        hint={meta([l.company, PORTAL_LABEL[l.portal], l.workMode && label(l.workMode), l.location, l.salary])}
        actions={
          <>
            <a href={l.url} target="_blank" rel="noopener noreferrer" className="adm-btn adm-btn--ghost">
              Open on {PORTAL_LABEL[l.portal]} ↗
            </a>
            {!['applied', 'applying', 'approved'].includes(l.status) && (
              <button type="button" className="adm-btn adm-btn--primary" disabled={busy} onClick={() => act(() => jobsApi.review(l.id, true), 'Approved')}>
                {l.status === 'failed' || l.status === 'dry_run' ? 'Apply again' : 'Approve'}
              </button>
            )}
            {!['applied', 'applying', 'skipped'].includes(l.status) && (
              <button type="button" className="adm-btn adm-btn--ghost" disabled={busy} onClick={() => act(() => jobsApi.review(l.id, false), 'Skipped')}>
                Skip
              </button>
            )}
            {['external', 'failed', 'needs_input', 'queued'].includes(l.status) && (
              <button
                type="button"
                className="adm-btn adm-btn--ghost"
                disabled={busy}
                onClick={() => act(() => jobsApi.markApplied(l.id).then(setData), 'Marked as applied')}
              >
                I applied myself
              </button>
            )}
          </>
        }
      />

      <div className="job-two">
        <Panel title="Match">
          <div className="job-match">
            <Score value={l.score} />
            <div>
              <p>{l.scoreReason || 'Not scored yet.'}</p>
              <p className="adm-row-sub">
                <StatusBadge status={l.status} />
                {l.statusNote && <span>{l.statusNote}</span>}
              </p>
            </div>
          </div>
          {!!d.matched?.length && <TagList title="You have" items={d.matched} />}
          {!!d.missing?.length && <TagList title="Missing or unclear" items={d.missing} />}
          {!!d.concerns?.length && <TagList title="Concerns" items={d.concerns} hot />}
        </Panel>
        <Panel title="Details">
          <dl className="adm-meta">
            <dt>Search</dt>
            <dd>{l.searchName || '—'}</dd>
            <dt>Type</dt>
            <dd>{l.jobType ? label(l.jobType) : '—'}</dd>
            <dt>Apply</dt>
            <dd>{l.easyApply === null ? 'Unknown' : l.easyApply ? 'On the portal' : 'On the company site'}</dd>
            <dt>Posted</dt>
            <dd>{formatDate(l.postedAt)}</dd>
            <dt>Found</dt>
            <dd>{formatDate(l.foundAt)}</dd>
            <dt>Applied</dt>
            <dd>{formatDate(l.appliedAt)}</dd>
          </dl>
        </Panel>
      </div>

      {l.status === 'needs_input' && latest && latest.pendingQuestions.length > 0 && (
        <QuestionsForm listingId={l.id} questions={latest.pendingQuestions} onDone={reload} />
      )}

      {applications.length > 0 && (
        <Panel title={`Attempts (${applications.length})`}>
          {applications.map((a) => (
            <Attempt key={a.id} a={a} />
          ))}
        </Panel>
      )}

      <Panel title="Description">
        <div className="job-desc">{l.description || <span className="adm-muted">No description was captured.</span>}</div>
      </Panel>
    </>
  );
}

function TagList({ title, items, hot }: { title: string; items: string[]; hot?: boolean }) {
  return (
    <div className="job-tags">
      <span className="adm-hint">{title}</span>
      <div>
        {items.map((t) => (
          <Badge key={t} tone={hot ? 'hot' : 'plain'}>
            {t}
          </Badge>
        ))}
      </div>
    </div>
  );
}

function QuestionsForm({ listingId, questions, onDone }: { listingId: number; questions: Question[]; onDone: () => void }) {
  const { errorText, notify } = useSession();
  const [values, setValues] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const answerable = questions.filter((q) => q.kind !== 'file');

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      const r = await jobsApi.answer(
        listingId,
        answerable.map((q) => ({ question: q.question, answer: values[q.key] ?? '' })),
      );
      notify(r.requeued ? 'Saved: the application is back in the queue' : 'Saved to your answer bank');
      onDone();
    } catch (err) {
      notify(errorText(err), 'error');
    } finally {
      setBusy(false);
    }
  }

  return (
    <Panel title="Questions the worker couldn't answer">
      <p className="adm-hint">Answers are saved to your answer bank and reused on every future form that asks the same thing.</p>
      <form className="adm-narrow" onSubmit={submit}>
        {questions.map((q) => (
          <div className="adm-field" key={q.key + q.question}>
            <label htmlFor={`q-${q.key}`}>{q.question}</label>
            {q.kind === 'file' ? (
              <p className="adm-muted">This form wants an extra file. Apply on the portal yourself, then mark it as applied.</p>
            ) : q.options && q.options.length > 0 ? (
              <select id={`q-${q.key}`} value={values[q.key] ?? ''} onChange={(e) => setValues({ ...values, [q.key]: e.target.value })}>
                <option value="">Choose…</option>
                {q.options.map((o) => (
                  <option key={o}>{o}</option>
                ))}
              </select>
            ) : q.kind === 'textarea' ? (
              <textarea id={`q-${q.key}`} rows={3} value={values[q.key] ?? ''} onChange={(e) => setValues({ ...values, [q.key]: e.target.value })} />
            ) : (
              <input id={`q-${q.key}`} value={values[q.key] ?? ''} onChange={(e) => setValues({ ...values, [q.key]: e.target.value })} />
            )}
          </div>
        ))}
        {answerable.length > 0 && (
          <button
            type="submit"
            className="adm-btn adm-btn--primary"
            disabled={busy || answerable.some((q) => !(values[q.key] ?? '').trim())}
          >
            {busy ? 'Saving…' : 'Save answers and retry'}
          </button>
        )}
      </form>
    </Panel>
  );
}

const APP_STATUS: Record<Application['status'], string> = {
  running: 'Running',
  submitted: 'Submitted',
  dry_run: 'Dry run (not submitted)',
  needs_input: 'Stopped for answers',
  external: 'External site',
  failed: 'Failed',
};

function Attempt({ a }: { a: Application }) {
  return (
    <article className="job-attempt">
      <header>
        <Badge tone={a.status === 'failed' || a.status === 'needs_input' ? 'hot' : 'plain'}>{APP_STATUS[a.status]}</Badge>
        <span className="adm-muted">
          {formatDate(a.startedAt)} · {a.steps} step{a.steps === 1 ? '' : 's'}
        </span>
      </header>
      {a.error && <p className="adm-error-text">{a.error}</p>}
      <div className="job-attempt-body">
        {a.screenshotUrl && (
          <a href={a.screenshotUrl} target="_blank" rel="noopener noreferrer" className="job-shot">
            <img src={a.screenshotUrl} alt="The form as the worker left it" loading="lazy" />
          </a>
        )}
        <div>
          {a.answers.length > 0 && (
            <dl className="adm-meta job-answers">
              {a.answers.map((x, i) => (
                <div key={i} className="job-answer">
                  <dt>{x.question}</dt>
                  <dd>
                    {x.answer} <span className="adm-muted">({x.source})</span>
                  </dd>
                </div>
              ))}
            </dl>
          )}
          {a.coverLetter && (
            <details>
              <summary>Cover letter</summary>
              <p className="job-desc">{a.coverLetter}</p>
            </details>
          )}
        </div>
      </div>
    </article>
  );
}
