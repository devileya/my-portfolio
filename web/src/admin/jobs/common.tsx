import { useEffect, useRef } from 'react';
import { STATUS_LABEL, label, type AccountStatus, type ListingStatus } from '../../lib/jobs-types';
import { Badge } from '../ui';

/** Re-runs fn every ms while the tab is visible. */
export function usePoll(fn: () => void, ms: number) {
  const ref = useRef(fn);
  ref.current = fn;
  useEffect(() => {
    const id = window.setInterval(() => {
      if (document.visibilityState === 'visible') ref.current();
    }, ms);
    return () => window.clearInterval(id);
  }, [ms]);
}

const HOT: ListingStatus[] = ['queued', 'needs_input', 'failed'];
const DIM: ListingStatus[] = ['filtered', 'skipped', 'new'];

export function StatusBadge({ status }: { status: ListingStatus }) {
  const tone = HOT.includes(status) ? 'hot' : DIM.includes(status) ? 'dim' : 'plain';
  return <Badge tone={tone}>{STATUS_LABEL[status] ?? status}</Badge>;
}

const ACCOUNT_TEXT: Record<AccountStatus, string> = {
  disconnected: 'Not connected',
  connected: 'Connected',
  expired: 'Session expired',
  challenge: 'Needs a human check',
  error: 'Error',
};

export function AccountBadge({ status, enabled = true }: { status: AccountStatus; enabled?: boolean }) {
  if (!enabled) return <Badge tone="dim">Off</Badge>;
  return <Badge tone={status === 'connected' ? 'plain' : status === 'disconnected' ? 'dim' : 'hot'}>{ACCOUNT_TEXT[status]}</Badge>;
}

export function Score({ value }: { value: number | null }) {
  if (value === null) return <span className="job-score job-score--none" aria-label="Not scored">—</span>;
  const band = value >= 85 ? 'top' : value >= 70 ? 'good' : value >= 55 ? 'mid' : 'low';
  return (
    <span className={`job-score job-score--${band}`} aria-label={`Match score ${value}`}>
      {value}
    </span>
  );
}

/** A set of toggle chips editing a string[]. */
export function Chips({
  legend,
  options,
  value,
  onChange,
  hint,
  format = label,
}: {
  legend: string;
  options: readonly string[];
  value: string[];
  onChange: (v: string[]) => void;
  hint?: string;
  format?: (v: string) => string;
}) {
  return (
    <fieldset className="job-chips">
      <legend>{legend}</legend>
      <div>
        {options.map((o) => {
          const on = value.includes(o);
          return (
            <button
              key={o}
              type="button"
              className={`job-chip${on ? ' is-on' : ''}`}
              aria-pressed={on}
              onClick={() => onChange(on ? value.filter((v) => v !== o) : [...value, o])}
            >
              {format(o)}
            </button>
          );
        })}
      </div>
      {hint && <p className="adm-hint">{hint}</p>}
    </fieldset>
  );
}

export function ago(iso: string | null | undefined) {
  if (!iso) return 'never';
  const s = Math.round((Date.now() - new Date(iso).getTime()) / 1000);
  if (s < -60) return `in ${until(-s)}`;
  if (s < 45) return 'just now';
  return `${until(s)} ago`;
}

function until(s: number) {
  if (s < 3600) return `${Math.max(1, Math.round(s / 60))} min`;
  if (s < 86400) return `${Math.round(s / 3600)} h`;
  return `${Math.round(s / 86400)} d`;
}

export function meta(parts: (string | null | undefined | false)[]) {
  return parts.filter(Boolean).join(' · ');
}
