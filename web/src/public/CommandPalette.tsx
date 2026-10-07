import { useEffect, useMemo, useRef, useState } from 'react';

export interface Command {
  id: string;
  label: string;
  hint?: string;
  run: () => void;
}

interface Props {
  open: boolean;
  onClose: () => void;
  commands: Command[];
}

/** ⌘K / Ctrl+K menu. A filtered list driven by the keyboard. */
export default function CommandPalette({ open, onClose, commands }: Props) {
  const [query, setQuery] = useState('');
  const [active, setActive] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const returnFocus = useRef<HTMLElement | null>(null);

  const results = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return commands;
    return commands
      .map((c) => ({ c, score: score(`${c.label} ${c.hint ?? ''}`.toLowerCase(), q) }))
      .filter((r) => r.score > 0)
      .sort((a, b) => b.score - a.score)
      .map((r) => r.c);
  }, [commands, query]);

  useEffect(() => {
    if (open) {
      returnFocus.current = document.activeElement as HTMLElement | null;
      setQuery('');
      setActive(0);
      requestAnimationFrame(() => inputRef.current?.focus());
    } else {
      returnFocus.current?.focus?.();
    }
  }, [open]);

  useEffect(() => setActive(0), [query]);

  if (!open) return null;

  const runAt = (i: number) => {
    const c = results[i];
    if (!c) return;
    onClose();
    // Let the dialog unmount before navigating or downloading.
    requestAnimationFrame(() => c.run());
  };

  return (
    <div className="palette-backdrop" onMouseDown={onClose}>
      <div
        className="palette"
        role="dialog"
        aria-modal="true"
        aria-label="Command menu"
        onMouseDown={(e) => e.stopPropagation()}
        onKeyDown={(e) => {
          if (e.key === 'Escape') onClose();
          else if (e.key === 'ArrowDown') {
            e.preventDefault();
            setActive((a) => Math.min(a + 1, results.length - 1));
          } else if (e.key === 'ArrowUp') {
            e.preventDefault();
            setActive((a) => Math.max(a - 1, 0));
          } else if (e.key === 'Enter') {
            e.preventDefault();
            runAt(active);
          } else if (e.key === 'Tab') {
            e.preventDefault(); // keep focus in the dialog
          }
        }}
      >
        <div className="palette__input">
          <span className="mono" aria-hidden>
            &gt;
          </span>
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Type a command or section"
            aria-label="Search commands"
            role="combobox"
            aria-expanded="true"
            aria-controls="palette-list"
            aria-activedescendant={results[active] ? `cmd-${results[active].id}` : undefined}
          />
          <kbd className="mono">esc</kbd>
        </div>
        <ul className="palette__list" id="palette-list" role="listbox">
          {results.length === 0 && <li className="palette__empty">No command matches “{query}”.</li>}
          {results.map((c, i) => (
            <li
              key={c.id}
              id={`cmd-${c.id}`}
              role="option"
              aria-selected={i === active}
              className={i === active ? 'is-active' : undefined}
              onMouseEnter={() => setActive(i)}
              onClick={() => runAt(i)}
            >
              <span>{c.label}</span>
              {c.hint && <span className="palette__hint mono">{c.hint}</span>}
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}

/** Subsequence match, rewarding contiguous runs and word starts. */
function score(text: string, q: string): number {
  if (text.includes(q)) return 100 - text.indexOf(q);
  let s = 0;
  let ti = 0;
  let run = 0;
  for (const ch of q) {
    const found = text.indexOf(ch, ti);
    if (found < 0) return 0;
    run = found === ti ? run + 1 : 1;
    s += run + (found === 0 || text[found - 1] === ' ' ? 2 : 0);
    ti = found + 1;
  }
  return s;
}
