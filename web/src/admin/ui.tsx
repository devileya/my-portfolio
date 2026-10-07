import { useEffect, useId, useRef, useState, type ReactNode } from 'react';
import { adminApi } from '../lib/api';
import { useSession } from './session';

// ── layout primitives ─────────────────────────────────────────────────────────

export function PageHeader({ title, hint, actions }: { title: string; hint?: string; actions?: ReactNode }) {
  return (
    <header className="adm-page-head">
      <div>
        <h1>{title}</h1>
        {hint && <p className="adm-hint">{hint}</p>}
      </div>
      {actions && <div className="adm-page-actions">{actions}</div>}
    </header>
  );
}

export function Panel({ title, children, actions }: { title?: string; children: ReactNode; actions?: ReactNode }) {
  return (
    <section className="adm-panel">
      {(title || actions) && (
        <div className="adm-panel-head">
          {title && <h2>{title}</h2>}
          {actions}
        </div>
      )}
      {children}
    </section>
  );
}

export function Loading({ label = 'Loading…' }: { label?: string }) {
  return <p className="adm-muted">{label}</p>;
}

export function ErrorNote({ text, onRetry }: { text: string; onRetry?: () => void }) {
  return (
    <div className="adm-error" role="alert">
      <span>{text}</span>
      {onRetry && (
        <button type="button" className="adm-btn adm-btn--ghost" onClick={onRetry}>
          Try again
        </button>
      )}
    </div>
  );
}

export function Badge({ children, tone = 'plain' }: { children: ReactNode; tone?: 'plain' | 'hot' | 'dim' }) {
  return <span className={`adm-badge adm-badge--${tone}`}>{children}</span>;
}

// ── form fields ───────────────────────────────────────────────────────────────

export function Field({
  label,
  hint,
  children,
  wide,
}: {
  label: string;
  hint?: string;
  children: (id: string) => ReactNode;
  wide?: boolean;
}) {
  const id = useId();
  return (
    <div className={`adm-field${wide ? ' adm-field--wide' : ''}`}>
      <label htmlFor={id}>{label}</label>
      {children(id)}
      {hint && <p className="adm-hint">{hint}</p>}
    </div>
  );
}

export function TextField({
  label,
  value,
  onChange,
  hint,
  type = 'text',
  wide,
  required,
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  hint?: string;
  type?: string;
  wide?: boolean;
  required?: boolean;
  placeholder?: string;
}) {
  return (
    <Field label={label} hint={hint} wide={wide}>
      {(id) => (
        <input
          id={id}
          type={type}
          value={value}
          required={required}
          placeholder={placeholder}
          onChange={(e) => onChange(e.target.value)}
        />
      )}
    </Field>
  );
}

export function TextArea({
  label,
  value,
  onChange,
  hint,
  rows = 4,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  hint?: string;
  rows?: number;
}) {
  return (
    <Field label={label} hint={hint} wide>
      {(id) => <textarea id={id} rows={rows} value={value} onChange={(e) => onChange(e.target.value)} />}
    </Field>
  );
}

export function Toggle({ label, checked, onChange }: { label: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <label className="adm-toggle">
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />
      <span className="adm-toggle-track" aria-hidden="true" />
      <span>{label}</span>
    </label>
  );
}

/** Comma-separated input that edits a string[]; keeps the raw text while typing. */
export function TagsField({
  label,
  value,
  onChange,
  hint,
}: {
  label: string;
  value: string[];
  onChange: (v: string[]) => void;
  hint?: string;
}) {
  const [text, setText] = useState(value.join(', '));
  const last = useRef(value);
  useEffect(() => {
    if (value !== last.current) {
      last.current = value;
      if (splitTags(text).join('|') !== value.join('|')) setText(value.join(', '));
    }
  }, [value, text]);
  return (
    <TextField
      label={label}
      hint={hint ?? 'Separate with commas.'}
      wide
      value={text}
      onChange={(t) => {
        setText(t);
        const tags = splitTags(t);
        last.current = tags;
        onChange(tags);
      }}
    />
  );
}

function splitTags(t: string) {
  return t
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean);
}

/** An ordered list of single-line strings with add / remove / move. */
export function ListEditor({
  label,
  hint,
  items,
  onChange,
  addLabel,
  multiline,
}: {
  label: string;
  hint?: string;
  items: string[];
  onChange: (v: string[]) => void;
  addLabel: string;
  multiline?: boolean;
}) {
  const set = (i: number, v: string) => onChange(items.map((x, j) => (j === i ? v : x)));
  return (
    <fieldset className="adm-list">
      <legend>{label}</legend>
      {hint && <p className="adm-hint">{hint}</p>}
      {items.length === 0 && <p className="adm-muted">Nothing here yet.</p>}
      <ol>
        {items.map((item, i) => (
          <li key={i}>
            <span className="adm-list-index">{i}</span>
            {multiline ? (
              <textarea rows={2} aria-label={`${label} ${i + 1}`} value={item} onChange={(e) => set(i, e.target.value)} />
            ) : (
              <input aria-label={`${label} ${i + 1}`} value={item} onChange={(e) => set(i, e.target.value)} />
            )}
            <MoveButtons
              index={i}
              count={items.length}
              onMove={(to) => onChange(move(items, i, to))}
              label={`${label} ${i + 1}`}
            />
            <button
              type="button"
              className="adm-icon-btn"
              aria-label={`Remove ${label.toLowerCase()} ${i + 1}`}
              onClick={() => onChange(items.filter((_, j) => j !== i))}
            >
              ×
            </button>
          </li>
        ))}
      </ol>
      <button type="button" className="adm-btn adm-btn--ghost" onClick={() => onChange([...items, ''])}>
        + {addLabel}
      </button>
    </fieldset>
  );
}

/** Rows of objects with fixed string columns (facts, socials). */
export function RowsEditor<T extends Record<string, string | undefined>>({
  label,
  rows,
  columns,
  onChange,
  blank,
  addLabel,
}: {
  label: string;
  rows: T[];
  columns: { key: keyof T & string; label: string; placeholder?: string }[];
  onChange: (v: T[]) => void;
  blank: T;
  addLabel: string;
}) {
  const set = (i: number, key: keyof T, v: string) => onChange(rows.map((r, j) => (j === i ? { ...r, [key]: v } : r)));
  return (
    <fieldset className={`adm-list adm-rows adm-rows--${columns.length}`}>
      <legend>{label}</legend>
      {rows.length === 0 && <p className="adm-muted">Nothing here yet.</p>}
      <ol>
        {rows.map((row, i) => (
          <li key={i}>
            {columns.map((c) => (
              <input
                key={c.key}
                aria-label={`${c.label} ${i + 1}`}
                placeholder={c.placeholder ?? c.label}
                value={row[c.key] ?? ''}
                onChange={(e) => set(i, c.key, e.target.value)}
              />
            ))}
            <MoveButtons index={i} count={rows.length} onMove={(to) => onChange(move(rows, i, to))} label={`${label} ${i + 1}`} />
            <button
              type="button"
              className="adm-icon-btn"
              aria-label={`Remove ${label.toLowerCase()} row ${i + 1}`}
              onClick={() => onChange(rows.filter((_, j) => j !== i))}
            >
              ×
            </button>
          </li>
        ))}
      </ol>
      <button type="button" className="adm-btn adm-btn--ghost" onClick={() => onChange([...rows, { ...blank }])}>
        + {addLabel}
      </button>
    </fieldset>
  );
}

export function MoveButtons({
  index,
  count,
  onMove,
  label,
  disabled,
}: {
  index: number;
  count: number;
  onMove: (to: number) => void;
  label: string;
  disabled?: boolean;
}) {
  return (
    <span className="adm-move">
      <button
        type="button"
        className="adm-icon-btn"
        aria-label={`Move ${label} up`}
        disabled={disabled || index === 0}
        onClick={() => onMove(index - 1)}
      >
        ↑
      </button>
      <button
        type="button"
        className="adm-icon-btn"
        aria-label={`Move ${label} down`}
        disabled={disabled || index === count - 1}
        onClick={() => onMove(index + 1)}
      >
        ↓
      </button>
    </span>
  );
}

export function move<T>(list: T[], from: number, to: number): T[] {
  if (to < 0 || to >= list.length) return list;
  const next = list.slice();
  const [item] = next.splice(from, 1);
  next.splice(to, 0, item);
  return next;
}

// ── actions ───────────────────────────────────────────────────────────────────

/** A destructive button that asks for confirmation in place, not in a browser dialog. */
export function ConfirmButton({
  label,
  confirmLabel,
  onConfirm,
  busy,
}: {
  label: string;
  confirmLabel: string;
  onConfirm: () => void;
  busy?: boolean;
}) {
  const [asking, setAsking] = useState(false);
  if (!asking) {
    return (
      <button type="button" className="adm-btn adm-btn--ghost adm-btn--danger" onClick={() => setAsking(true)} disabled={busy}>
        {label}
      </button>
    );
  }
  return (
    <span className="adm-confirm" role="group" aria-label="Confirm">
      <button
        type="button"
        className="adm-btn adm-btn--danger"
        disabled={busy}
        autoFocus
        onClick={() => {
          setAsking(false);
          onConfirm();
        }}
      >
        {confirmLabel}
      </button>
      <button type="button" className="adm-btn adm-btn--ghost" onClick={() => setAsking(false)}>
        Cancel
      </button>
    </span>
  );
}

/** Uploads an image to storage and reports its key and URL. */
export function ImageUpload({
  label,
  url,
  onChange,
}: {
  label: string;
  url: string;
  onChange: (key: string, url: string) => void;
}) {
  const { errorText } = useSession();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const input = useRef<HTMLInputElement>(null);
  const id = useId();

  async function pick(file: File | undefined) {
    if (!file) return;
    setBusy(true);
    setError('');
    try {
      const obj = await adminApi.uploadImage(file);
      onChange(obj.key, obj.url);
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
      if (input.current) input.current.value = '';
    }
  }

  return (
    <div className="adm-field adm-field--wide">
      <label htmlFor={id}>{label}</label>
      <div className="adm-image">
        <div className="adm-image-preview">{url ? <img src={url} alt="" /> : <span className="adm-muted">No image</span>}</div>
        <div className="adm-image-actions">
          <input
            ref={input}
            id={id}
            type="file"
            accept="image/png,image/jpeg,image/webp,image/gif,image/avif"
            className="sr-only"
            onChange={(e) => pick(e.target.files?.[0])}
          />
          <button type="button" className="adm-btn adm-btn--ghost" disabled={busy} onClick={() => input.current?.click()}>
            {busy ? 'Uploading…' : url ? 'Replace image' : 'Upload image'}
          </button>
          {url && !busy && (
            <button type="button" className="adm-btn adm-btn--ghost" onClick={() => onChange('', '')}>
              Remove image
            </button>
          )}
          <p className="adm-hint">PNG, JPEG, WebP, GIF or AVIF, up to 10 MB.</p>
          {error && <p className="adm-error-text" role="alert">{error}</p>}
        </div>
      </div>
    </div>
  );
}

// ── data loading ──────────────────────────────────────────────────────────────

/** Loads data once on mount; `reload` re-runs it. Errors go through errorText (so 401s sign out). */
export function useLoad<T>(fn: () => Promise<T>) {
  const { errorText } = useSession();
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState('');
  const [tick, setTick] = useState(0);
  const fnRef = useRef(fn);
  fnRef.current = fn;

  useEffect(() => {
    let live = true;
    setError('');
    fnRef
      .current()
      .then((d) => live && setData(d))
      .catch((err) => live && setError(errorText(err)));
    return () => {
      live = false;
    };
  }, [tick, errorText]);

  return { data, setData, error, reload: () => setTick((t) => t + 1) };
}

export function formatBytes(n: number) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

export function formatDate(iso: string | null | undefined) {
  if (!iso) return '—';
  return new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
}
