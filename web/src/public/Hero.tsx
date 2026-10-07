import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { Profile } from '../lib/types';
import { attention, buildTokens, magma, sample } from './inference';

interface Props {
  profile: Profile;
  /** Lets the command palette trigger a regeneration. */
  regenerateSignal: number;
}

const prefersReducedMotion = () =>
  typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

export default function Hero({ profile, regenerateSignal }: Props) {
  const headlines = useMemo(
    () => (profile.headlines.length ? profile.headlines : [profile.summary || profile.role]),
    [profile.headlines, profile.summary, profile.role],
  );
  const [temperature, setTemperature] = useState(0);
  const [pick, setPick] = useState(0);
  const [run, setRun] = useState(0); // bumps on every generation, even if pick is unchanged
  const [emitted, setEmitted] = useState(0);
  const [hovered, setHovered] = useState<number | null>(null);
  const [elapsed, setElapsed] = useState(0);
  const startRef = useRef(0);

  const tokens = useMemo(() => buildTokens(headlines, pick), [headlines, pick]);
  const attn = useMemo(() => attention(tokens), [tokens]);
  const done = emitted >= tokens.length;

  // Stream tokens with a little jitter, like a real decode loop.
  useEffect(() => {
    if (prefersReducedMotion()) {
      setEmitted(tokens.length);
      setElapsed(0);
      return;
    }
    setEmitted(0);
    let i = 0;
    let timer: number;
    const step = () => {
      if (i === 0) startRef.current = performance.now();
      i += 1;
      setEmitted(i);
      if (i < tokens.length) {
        timer = window.setTimeout(step, 28 + Math.random() * 55);
      } else {
        setElapsed(performance.now() - startRef.current);
      }
    };
    timer = window.setTimeout(step, run === 0 ? 650 : 120);
    return () => window.clearTimeout(timer);
  }, [tokens, run]);

  const regenerate = useCallback(() => {
    setHovered(null);
    setPick((current) => sample(headlines.length, temperature, temperature > 0 ? current : undefined));
    setRun((r) => r + 1);
  }, [headlines.length, temperature]);

  useEffect(() => {
    if (regenerateSignal > 0) regenerate();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [regenerateSignal]);

  const focusRow = hovered ?? (done ? null : emitted - 1);
  const tps = done && elapsed > 0 ? Math.round((tokens.length / elapsed) * 1000) : null;

  return (
    <section className="hero" id="top" aria-label="Introduction">
      <div className="hero__grid">
        <div className="hero__main">
          <p className="hero__prompt mono">
            <span className="hero__caret" aria-hidden>
              &gt;
            </span>{' '}
            {renderPrompt(profile.prompt || `describe ${profile.name.toLowerCase()} in one sentence`, profile.name)}
          </p>

          <h1 className="hero__output mono" aria-label={headlines[pick]} onMouseLeave={() => setHovered(null)}>
            {tokens.map((t, i) => {
              const visible = i < emitted;
              const weight = focusRow !== null && i <= focusRow ? attn[focusRow][i] : 0;
              return (
                <span
                  key={`${run}-${i}`}
                  aria-hidden
                  className={`tok${visible ? ' is-on' : ''}${hovered === i ? ' is-hovered' : ''}${!done && i === emitted - 1 ? ' is-last' : ''}`}
                  style={
                    {
                      '--p': magma(0.25 + t.p * 0.75),
                      '--attn': weight > 0.05 && hovered !== null ? magma(weight * 0.85) : 'transparent',
                      // bright cells need dark ink to stay legible
                      color: hovered !== null && weight * 0.85 > 0.6 ? 'var(--ink-0)' : undefined,
                    } as React.CSSProperties
                  }
                  onMouseEnter={() => visible && setHovered(i)}
                  onClick={() => visible && setHovered((h) => (h === i ? null : i))}
                >
                  {t.text}
                  {hovered === i && <TokenTip token={t} index={i} />}
                </span>
              );
            })}
            {emitted === 0 && <span className="hero__cursor" aria-hidden />}
          </h1>

          <div className="hero__who">
            <span className="hero__name">{profile.name}</span>
            <span className="hero__role">{profile.role}</span>
          </div>

          <div className="hero__actions">
            {profile.cv ? (
              <a className="btn btn--primary" href="/api/cv" download>
                Download CV
              </a>
            ) : (
              <a className="btn btn--primary" href="#cv">
                Get my CV
              </a>
            )}
            <a className="btn" href="#work">
              See the work
            </a>
          </div>
        </div>

        <aside className="hero__side" aria-label="Generation controls">
          <AttentionMap tokens={tokens.map((t) => t.text.trim())} matrix={attn} emitted={emitted} focusRow={focusRow} onHover={setHovered} />

          <div className="hero__controls">
            <label className="temp">
              <span className="temp__label mono">
                temperature <output>{temperature.toFixed(1)}</output>
              </span>
              <input
                type="range"
                min={0}
                max={1.5}
                step={0.1}
                value={temperature}
                onChange={(e) => setTemperature(Number(e.target.value))}
                style={{ '--fill': `${(temperature / 1.5) * 100}%` } as React.CSSProperties}
              />
            </label>
            <button type="button" className="btn btn--small" onClick={regenerate}>
              Regenerate
            </button>
          </div>
          <p className="hero__meta mono" aria-live="polite">
            {done
              ? `${tokens.length} tokens${tps ? ` · ${tps} tok/s` : ''} · ${temperature === 0 ? 'greedy' : `sampled at T=${temperature.toFixed(1)}`}`
              : `generating… ${emitted}/${tokens.length}`}
          </p>
          {temperature === 0 && done && run > 0 && (
            <p className="hero__hint">At T=0 the answer is deterministic. Raise the temperature to sample another.</p>
          )}
        </aside>
      </div>
    </section>
  );
}

function renderPrompt(prompt: string, name: string) {
  const i = prompt.toLowerCase().indexOf(name.toLowerCase());
  if (i < 0) return prompt;
  return (
    <>
      {prompt.slice(0, i)}
      <mark>{prompt.slice(i, i + name.length)}</mark>
      {prompt.slice(i + name.length)}
    </>
  );
}

function TokenTip({ token, index }: { token: ReturnType<typeof buildTokens>[number]; index: number }) {
  return (
    <span className="tok-tip mono" role="tooltip">
      <span className="tok-tip__head">
        token {index} · p={token.p.toFixed(2)}
      </span>
      <span className="tok-tip__row">
        <Bar p={token.p} />
        <span className="tok-tip__text">{token.text.trim()}</span>
      </span>
      {token.alternatives.map((a) => (
        <span className="tok-tip__row is-alt" key={a.text}>
          <Bar p={a.p} />
          <span className="tok-tip__text">{a.text}</span>
        </span>
      ))}
    </span>
  );
}

function Bar({ p }: { p: number }) {
  return (
    <span className="pbar">
      <span style={{ width: `${Math.max(4, p * 100)}%`, background: magma(0.3 + p * 0.7) }} />
    </span>
  );
}

interface MapProps {
  tokens: string[];
  matrix: number[][];
  emitted: number;
  focusRow: number | null;
  onHover: (i: number | null) => void;
}

function AttentionMap({ tokens, matrix, emitted, focusRow, onHover }: MapProps) {
  const n = tokens.length;
  return (
    <figure className="attn" aria-label="Attention map of the generated sentence">
      <figcaption className="attn__cap mono">
        <span>causal self-attention</span>
        <span className="attn__legend" aria-hidden>
          <i style={{ background: magma(0.05) }} />
          <i style={{ background: magma(0.35) }} />
          <i style={{ background: magma(0.6) }} />
          <i style={{ background: magma(0.85) }} />
        </span>
      </figcaption>
      <div
        className="attn__grid"
        style={{ gridTemplateColumns: `repeat(${n}, 1fr)` }}
        onMouseLeave={() => onHover(null)}
        aria-hidden
      >
        {matrix.map((row, i) =>
          row.map((v, j) => {
            const on = i < emitted && j <= i;
            return (
              <span
                key={`${i}-${j}`}
                className={`attn__cell${focusRow === i && j <= i ? ' is-row' : ''}`}
                style={{ background: on ? magma(v * 0.92) : 'transparent' }}
                onMouseEnter={() => i < emitted && onHover(i)}
                title={on ? `${tokens[i]} → ${tokens[j]}: ${v.toFixed(2)}` : undefined}
              />
            );
          }),
        )}
      </div>
      <p className="attn__note">Hover a word to see what it attends to.</p>
    </figure>
  );
}
