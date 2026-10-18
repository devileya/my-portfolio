import { useState, type FormEvent } from 'react';
import { jobsApi } from '../../lib/api';
import { PORTALS, PORTAL_LABEL, type AIProvider, type JobSettings, type TelegramInfo } from '../../lib/jobs-types';
import { useSession } from '../session';
import { Badge, ConfirmButton, ErrorNote, Field, Loading, PageHeader, Panel, TextArea, TextField, Toggle, useLoad } from '../ui';

const PRESETS: { name: string; kind: AIProvider['kind']; baseUrl: string; model: string }[] = [
  { name: 'Anthropic', kind: 'anthropic', baseUrl: '', model: 'claude-opus-5' },
  { name: 'OpenAI', kind: 'openai', baseUrl: 'https://api.openai.com/v1', model: '' },
  { name: 'Vikey', kind: 'openai', baseUrl: 'https://api.vikey.ai/v1', model: 'qwen/qwen3.8-max' },
  { name: 'OpenRouter', kind: 'openai', baseUrl: 'https://openrouter.ai/api/v1', model: '' },
  { name: 'Google Gemini', kind: 'openai', baseUrl: 'https://generativelanguage.googleapis.com/v1beta/openai', model: '' },
  { name: 'DeepSeek', kind: 'openai', baseUrl: 'https://api.deepseek.com/v1', model: 'deepseek-chat' },
  { name: 'Other (OpenAI-compatible)', kind: 'openai', baseUrl: '', model: '' },
];

const APPLICANT_FIELDS: { key: string; label: string; placeholder?: string }[] = [
  { key: 'phone', label: 'Phone', placeholder: '+62 812…' },
  { key: 'city', label: 'City', placeholder: 'Jakarta' },
  { key: 'country', label: 'Country', placeholder: 'Indonesia' },
  { key: 'expectedSalary', label: 'Expected salary (monthly)', placeholder: 'IDR 35,000,000' },
  { key: 'currentSalary', label: 'Current salary (monthly)' },
  { key: 'noticePeriod', label: 'Notice period', placeholder: '1 month' },
  { key: 'workAuthorization', label: 'Work authorization', placeholder: 'Indonesian citizen; needs sponsorship abroad' },
  { key: 'willingToRelocate', label: 'Willing to relocate', placeholder: 'Yes, within Asia' },
  { key: 'highestEducation', label: 'Highest education', placeholder: "Bachelor's in Computer Science" },
  { key: 'languages', label: 'Languages', placeholder: 'Indonesian (native), English (professional)' },
  { key: 'githubUrl', label: 'GitHub URL' },
  { key: 'linkedinUrl', label: 'LinkedIn URL' },
  { key: 'gender', label: 'Gender (only if forms require it)' },
  { key: 'dateOfBirth', label: 'Date of birth (only if forms require it)' },
];

export default function JobSettingsPage() {
  const settings = useLoad(jobsApi.settings);
  const providers = useLoad(jobsApi.providers);

  return (
    <>
      <PageHeader title="Job Hunter settings" />
      <ProvidersPanel list={providers.data} error={providers.error} reload={providers.reload} setList={providers.setData} />
      {settings.data && <TelegramPanel initial={settings.data.telegram} />}
      {settings.error && !settings.data && <ErrorNote text={settings.error} onRetry={settings.reload} />}
      {!settings.data && !settings.error && <Loading />}
      {settings.data && <SettingsForm initial={settings.data} providers={providers.data ?? []} onSaved={settings.setData} />}
    </>
  );
}

function SettingsForm({ initial, providers, onSaved }: { initial: JobSettings; providers: AIProvider[]; onSaved: (s: JobSettings) => void }) {
  const { errorText, notify } = useSession();
  const [s, setS] = useState(initial);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const dirty = JSON.stringify(s) !== JSON.stringify(initial);
  const set = <K extends keyof JobSettings>(k: K, v: JobSettings[K]) => setS((cur) => ({ ...cur, [k]: v }));
  const num = (v: string) => Math.max(0, Number.parseInt(v || '0', 10) || 0);

  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const saved = await jobsApi.saveSettings(s);
      setS(saved);
      onSaved(saved);
      notify('Settings saved');
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  const providerSelect = (id: string, value: number | null, onChange: (v: number | null) => void, none: string) => (
    <select id={id} value={value ?? ''} onChange={(e) => onChange(e.target.value ? Number(e.target.value) : null)}>
      <option value="">{none}</option>
      {providers.map((p) => (
        <option key={p.id} value={p.id}>
          {p.label} · {p.model}
        </option>
      ))}
    </select>
  );

  return (
    <form onSubmit={save}>
      {error && <ErrorNote text={error} />}

      <Panel title="Which model does what">
        <div className="adm-grid">
          <Field label="Scoring jobs" hint="Runs once per new listing. A fast, cheap model is fine.">
            {(id) => providerSelect(id, s.scoreProviderId, (v) => set('scoreProviderId', v), 'Choose a provider…')}
          </Field>
          <Field label="Filling forms & cover letters" hint="Fewer calls, more care. Leave empty to use the scoring model.">
            {(id) => providerSelect(id, s.applyProviderId, (v) => set('applyProviderId', v), 'Same as scoring')}
          </Field>
        </div>
      </Panel>

      <Panel title="Automation">
        <div className="adm-grid">
          <div className="adm-field adm-field--toggle adm-field--wide">
            <Toggle label="Paused (no searches or applications)" checked={s.paused} onChange={(v) => set('paused', v)} />
            <Toggle label="Dry run: fill forms but never press Submit" checked={s.dryRun} onChange={(v) => set('dryRun', v)} />
          </div>
          <TextField label="Time zone" value={s.timezone} onChange={(v) => set('timezone', v)} hint="IANA name, e.g. Asia/Jakarta." />
          <div className="adm-field">
            <label>Apply between (hours)</label>
            <div className="job-inline">
              <input aria-label="From hour" type="number" min={0} max={23} value={s.applyStartHour} onChange={(e) => set('applyStartHour', num(e.target.value))} />
              <span>to</span>
              <input aria-label="To hour" type="number" min={1} max={24} value={s.applyEndHour} onChange={(e) => set('applyEndHour', num(e.target.value))} />
            </div>
          </div>
          <div className="adm-field">
            <label>Gap between applications on one portal (minutes)</label>
            <div className="job-inline">
              <input aria-label="Minimum gap" type="number" min={1} value={s.minGapMinutes} onChange={(e) => set('minGapMinutes', num(e.target.value))} />
              <span>to</span>
              <input aria-label="Maximum gap" type="number" min={1} value={s.maxGapMinutes} onChange={(e) => set('maxGapMinutes', num(e.target.value))} />
            </div>
            <p className="adm-hint">A random gap in this range keeps the pace human.</p>
          </div>
          <div className="adm-field adm-field--wide">
            <label>Daily caps per portal</label>
            <div className="job-caps">
              {PORTALS.map((p) => (
                <label key={p}>
                  <span>{PORTAL_LABEL[p]}</span>
                  <input
                    type="number"
                    min={0}
                    value={s.portalCaps[p] ?? 0}
                    onChange={(e) => set('portalCaps', { ...s.portalCaps, [p]: num(e.target.value) })}
                  />
                </label>
              ))}
            </div>
            <p className="adm-hint">0 means no cap. LinkedIn is the strictest about volume; 10–15 a day is a sensible ceiling.</p>
          </div>
        </div>
      </Panel>

      <Panel title="Facts for application forms">
        <p className="adm-hint">
          Name, email, location, experience and skills come from your portfolio profile. Add what forms ask that the profile doesn’t say.
          Leave anything you’d rather answer case by case empty.
        </p>
        <div className="adm-grid">
          {APPLICANT_FIELDS.map((f) => (
            <TextField
              key={f.key}
              label={f.label}
              placeholder={f.placeholder}
              value={s.applicant[f.key] ?? ''}
              onChange={(v) => set('applicant', { ...s.applicant, [f.key]: v })}
            />
          ))}
          <TextArea
            label="Cover letter guidance"
            value={s.coverLetterStyle}
            onChange={(v) => set('coverLetterStyle', v)}
            rows={3}
            hint="Tone and points to stress, e.g. “Concise, confident. Lead with AI/LLM production work; mention open to relocation.”"
          />
        </div>
      </Panel>

      <Panel title="What Telegram tells you">
        <div className="adm-grid">
          <div className="adm-field adm-field--toggle adm-field--wide">
            <Toggle label="New matches (with Apply / Skip buttons)" checked={s.notifyMatches} onChange={(v) => set('notifyMatches', v)} />
            <Toggle label="Each application sent" checked={s.notifyApplied} onChange={(v) => set('notifyApplied', v)} />
            <Toggle label="Daily summary" checked={s.notifyDigest} onChange={(v) => set('notifyDigest', v)} />
          </div>
          <TextField label="Daily summary at (hour)" type="number" value={String(s.digestHour)} onChange={(v) => set('digestHour', Math.min(23, num(v)))} />
        </div>
        <p className="adm-hint">
          Questions the worker can’t answer and session problems are always sent. In the chat: /status, /queue, /run, /pause, /resume.
        </p>
      </Panel>

      <div className="adm-savebar">
        <span className="adm-muted">{dirty ? 'You have unsaved changes.' : 'All changes saved.'}</span>
        <button type="submit" className="adm-btn adm-btn--primary" disabled={busy || !dirty}>
          {busy ? 'Saving…' : 'Save settings'}
        </button>
      </div>
    </form>
  );
}

// ── telegram ──────────────────────────────────────────────────────────────────

function TelegramPanel({ initial }: { initial: TelegramInfo }) {
  const { errorText, notify } = useSession();
  const [t, setT] = useState(initial);
  const [token, setToken] = useState('');
  const [replacing, setReplacing] = useState(!initial.hasToken);
  const [chats, setChats] = useState<{ id: number; name: string }[] | null>(null);
  const [manualId, setManualId] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  async function run(fn: () => Promise<JobSettings | void>, done?: string) {
    setBusy(true);
    setError('');
    try {
      const r = await fn();
      if (r) setT(r.telegram);
      if (done) notify(done);
      return true;
    } catch (err) {
      setError(errorText(err));
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function saveToken(e: FormEvent) {
    e.preventDefault();
    if (await run(() => jobsApi.saveTelegramToken(token), 'Bot token saved')) {
      setToken('');
      setReplacing(false);
      setChats(null);
    }
  }

  async function findChats() {
    setBusy(true);
    setError('');
    try {
      const list = await jobsApi.telegramChats();
      setChats(list);
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  const ready = t.hasToken && t.chatId !== 0;

  return (
    <Panel
      title="Telegram bot"
      actions={ready && <Badge>Connected</Badge>}
    >
      {error && <ErrorNote text={error} />}

      <ol className="job-steps">
        <li className={t.hasToken && !replacing ? 'is-done' : ''}>
          <strong>Bot token</strong>
          {t.hasToken && !replacing ? (
            <div className="adm-row-sub">
              <span>
                @{t.botName} · token {t.tokenHint}
              </span>
              <button type="button" className="adm-btn adm-btn--ghost" onClick={() => setReplacing(true)}>
                Replace
              </button>
              <ConfirmButton label="Disconnect" confirmLabel="Disconnect bot" busy={busy} onConfirm={() => run(() => jobsApi.clearTelegram(), 'Bot disconnected')} />
            </div>
          ) : (
            <form className="job-inline-form" onSubmit={saveToken}>
              <p className="adm-hint">
                In Telegram, open <a href="https://t.me/BotFather" target="_blank" rel="noopener noreferrer">@BotFather</a>, send{' '}
                <code>/newbot</code>, pick a name, and paste the token it gives you. Use a new bot just for Job Hunter.
              </p>
              <div className="job-inline">
                <input
                  type="password"
                  aria-label="Bot token"
                  placeholder="123456789:AA…"
                  value={token}
                  onChange={(e) => setToken(e.target.value)}
                  autoComplete="off"
                />
                <button type="submit" className="adm-btn adm-btn--primary" disabled={busy || !token.trim()}>
                  {busy ? 'Checking…' : 'Save token'}
                </button>
                {t.hasToken && (
                  <button type="button" className="adm-btn adm-btn--ghost" onClick={() => setReplacing(false)}>
                    Cancel
                  </button>
                )}
              </div>
            </form>
          )}
        </li>

        <li className={ready ? 'is-done' : ''}>
          <strong>Your chat</strong>
          {!t.hasToken ? (
            <p className="adm-muted">Save the token first.</p>
          ) : t.chatId !== 0 && chats === null ? (
            <div className="adm-row-sub">
              <span>Sending to {t.chatName || t.chatId}</span>
              <button
                type="button"
                className="adm-btn adm-btn--ghost"
                disabled={busy}
                onClick={() => run(() => jobsApi.saveTelegramChat(0, '', t.poll)).then((ok) => ok && setChats([]))}
              >
                Change
              </button>
            </div>
          ) : (
            <div>
              <p className="adm-hint">
                Open{' '}
                <a href={`https://t.me/${t.botName}`} target="_blank" rel="noopener noreferrer">
                  t.me/{t.botName}
                </a>
                , press <b>Start</b> (or send any message), then find your chat here.
              </p>
              <div className="job-inline">
                <button type="button" className="adm-btn adm-btn--primary" disabled={busy} onClick={findChats}>
                  {busy ? 'Looking…' : 'Find my chat'}
                </button>
              </div>
              {chats && chats.length === 0 && <p className="adm-muted">No messages yet. Send the bot a message, then try again.</p>}
              {chats && chats.length > 0 && (
                <ul className="job-chat-list">
                  {chats.map((c) => (
                    <li key={c.id}>
                      <span>{c.name || c.id}</span>
                      <button
                        type="button"
                        className="adm-btn adm-btn--ghost"
                        disabled={busy}
                        onClick={async () => {
                          if (await run(() => jobsApi.saveTelegramChat(c.id, c.name, t.poll), 'Chat connected: try Send test')) setChats(null);
                        }}
                      >
                        Use this chat
                      </button>
                    </li>
                  ))}
                </ul>
              )}
              <details className="job-manual">
                <summary>Enter a chat ID instead</summary>
                <div className="job-inline">
                  <input aria-label="Chat ID" inputMode="numeric" value={manualId} onChange={(e) => setManualId(e.target.value)} placeholder="e.g. 123456789" />
                  <button
                    type="button"
                    className="adm-btn adm-btn--ghost"
                    disabled={busy || !/^-?\d+$/.test(manualId.trim())}
                    onClick={async () => {
                      if (await run(() => jobsApi.saveTelegramChat(Number(manualId.trim()), '', t.poll), 'Chat saved')) setChats(null);
                    }}
                  >
                    Save
                  </button>
                </div>
              </details>
            </div>
          )}
        </li>

        <li className={ready ? 'is-done' : ''}>
          <strong>Check it</strong>
          <div className="adm-row-sub">
            <button type="button" className="adm-btn adm-btn--ghost" disabled={busy || !ready} onClick={() => run(() => jobsApi.testTelegram(), 'Test message sent')}>
              Send test message
            </button>
            <Toggle
              label="Receive button presses and replies"
              checked={t.poll}
              onChange={(v) => run(() => jobsApi.saveTelegramChat(t.chatId, t.chatName, v), v ? 'Listening for replies' : 'Notifications only')}
            />
          </div>
          <p className="adm-hint">
            Keep this on so ✅ Apply / ⏭ Skip and replies to ❓ questions work. Turn it off only if another program already reads
            this bot’s messages.
          </p>
        </li>
      </ol>
    </Panel>
  );
}

// ── providers ─────────────────────────────────────────────────────────────────

function ProvidersPanel({
  list,
  error,
  reload,
  setList,
}: {
  list: AIProvider[] | null;
  error: string;
  reload: () => void;
  setList: (v: AIProvider[]) => void;
}) {
  const [adding, setAdding] = useState(false);
  return (
    <Panel
      title="AI providers"
      actions={
        !adding && (
          <button type="button" className="adm-btn adm-btn--ghost" onClick={() => setAdding(true)}>
            + Add provider
          </button>
        )
      }
    >
      {error && <ErrorNote text={error} onRetry={reload} />}
      {list && list.length === 0 && !adding && (
        <p className="adm-muted">No providers yet. Add Anthropic, OpenAI, Vikey or any OpenAI-compatible API. Keys are stored encrypted.</p>
      )}
      {list && list.length > 0 && (
        <ul className="job-provider-list">
          {list.map((p) => (
            <ProviderRow
              key={p.id}
              p={p}
              onSaved={(np) => setList(list.map((x) => (x.id === np.id ? np : x)))}
              onDeleted={() => setList(list.filter((x) => x.id !== p.id))}
            />
          ))}
        </ul>
      )}
      {adding && (
        <ProviderForm
          onCancel={() => setAdding(false)}
          onSaved={(p) => {
            setList([...(list ?? []), p]);
            setAdding(false);
            reload();
          }}
        />
      )}
    </Panel>
  );
}

function ProviderRow({ p, onSaved, onDeleted }: { p: AIProvider; onSaved: (p: AIProvider) => void; onDeleted: () => void }) {
  const { errorText, notify } = useSession();
  const [editing, setEditing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState('');

  async function test() {
    setBusy(true);
    setResult('');
    try {
      const r = await jobsApi.testProvider(p.id);
      setResult(`Works: answered in ${(r.ms / 1000).toFixed(1)} s.`);
    } catch (err) {
      setResult(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    setBusy(true);
    try {
      await jobsApi.deleteProvider(p.id);
      onDeleted();
    } catch (err) {
      notify(errorText(err), 'error');
      setBusy(false);
    }
  }

  if (editing) {
    return (
      <li>
        <ProviderForm
          initial={p}
          onCancel={() => setEditing(false)}
          onSaved={(np) => {
            onSaved(np);
            setEditing(false);
          }}
        />
      </li>
    );
  }
  return (
    <li>
      <div className="adm-row-main">
        <strong>{p.label}</strong>
        <div className="adm-row-sub">
          <Badge>{p.kind === 'anthropic' ? 'Anthropic' : 'OpenAI-compatible'}</Badge>
          <code>{p.model}</code>
          {p.baseUrl && <span className="adm-muted">{p.baseUrl}</span>}
          <span className="adm-muted">key {p.hasKey ? p.keyHint : 'missing'}</span>
        </div>
        {result && <p className={result.startsWith('Works') ? 'adm-hint' : 'adm-error-text'}>{result}</p>}
      </div>
      <span className="adm-row-actions">
        <button type="button" className="adm-btn adm-btn--ghost" disabled={busy} onClick={test}>
          {busy ? 'Testing…' : 'Test'}
        </button>
        <button type="button" className="adm-btn adm-btn--ghost" onClick={() => setEditing(true)}>
          Edit
        </button>
        <ConfirmButton label="Delete" confirmLabel="Delete provider" busy={busy} onConfirm={remove} />
      </span>
    </li>
  );
}

function ProviderForm({ initial, onSaved, onCancel }: { initial?: AIProvider; onSaved: (p: AIProvider) => void; onCancel: () => void }) {
  const { errorText } = useSession();
  const [v, setV] = useState({
    label: initial?.label ?? 'Anthropic',
    kind: initial?.kind ?? ('anthropic' as AIProvider['kind']),
    baseUrl: initial?.baseUrl ?? '',
    model: initial?.model ?? 'claude-opus-5',
    apiKey: '',
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [models, setModels] = useState<string[] | null>(null);

  function preset(name: string) {
    const p = PRESETS.find((x) => x.name === name);
    if (p) setV({ ...v, label: p.name.replace(/ \(.*\)$/, ''), kind: p.kind, baseUrl: p.baseUrl, model: p.model });
  }

  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const saved = initial ? await jobsApi.updateProvider({ ...v, id: initial.id }) : await jobsApi.createProvider(v);
      onSaved(saved);
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  async function listModels() {
    if (!initial) return;
    try {
      setModels(await jobsApi.providerModels(initial.id));
    } catch (err) {
      setError(errorText(err));
    }
  }

  return (
    <form className="job-provider-form" onSubmit={save}>
      {error && <ErrorNote text={error} />}
      <div className="adm-grid">
        {!initial && (
          <Field label="Preset">
            {(id) => (
              <select id={id} defaultValue="Anthropic" onChange={(e) => preset(e.target.value)}>
                {PRESETS.map((p) => (
                  <option key={p.name}>{p.name}</option>
                ))}
              </select>
            )}
          </Field>
        )}
        <TextField label="Name" value={v.label} onChange={(x) => setV({ ...v, label: x })} required />
        <Field label="API type">
          {(id) => (
            <select id={id} value={v.kind} onChange={(e) => setV({ ...v, kind: e.target.value as AIProvider['kind'] })}>
              <option value="anthropic">Anthropic Messages API</option>
              <option value="openai">OpenAI-compatible (/chat/completions)</option>
            </select>
          )}
        </Field>
        <TextField
          label="Base URL"
          value={v.baseUrl}
          onChange={(x) => setV({ ...v, baseUrl: x })}
          placeholder={v.kind === 'anthropic' ? 'Default (api.anthropic.com)' : 'https://…/v1'}
        />
        <Field label="Model" hint={initial ? undefined : 'After saving, “List models” shows what the key can use.'}>
          {(id) => (
            <>
              <input id={id} value={v.model} list={models ? `models-${initial?.id}` : undefined} onChange={(e) => setV({ ...v, model: e.target.value })} required />
              {models && (
                <datalist id={`models-${initial?.id}`}>
                  {models.map((m) => (
                    <option key={m} value={m} />
                  ))}
                </datalist>
              )}
            </>
          )}
        </Field>
        <TextField
          label="API key"
          type="password"
          value={v.apiKey}
          onChange={(x) => setV({ ...v, apiKey: x })}
          placeholder={initial?.hasKey ? `Stored (${initial.keyHint}); leave empty to keep` : ''}
          required={!initial}
        />
      </div>
      <div className="adm-row-actions">
        <button type="submit" className="adm-btn adm-btn--primary" disabled={busy}>
          {busy ? 'Saving…' : initial ? 'Save provider' : 'Add provider'}
        </button>
        {initial && (
          <button type="button" className="adm-btn adm-btn--ghost" onClick={listModels}>
            List models
          </button>
        )}
        <button type="button" className="adm-btn adm-btn--ghost" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </form>
  );
}
