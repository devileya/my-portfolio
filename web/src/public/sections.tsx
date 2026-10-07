import { useEffect, useMemo, useRef, useState } from 'react';
import { publicApi, ApiError } from '../lib/api';
import { CATEGORY_LABEL, SKILL_LEVELS } from '../lib/types';
import type { Category, Experience, Profile, Project, Skill } from '../lib/types';
import { magma } from './inference';

// ── top bar ───────────────────────────────────────────────────────────────────

export function TopBar({ profile, onOpenPalette }: { profile: Profile; onOpenPalette: () => void }) {
  const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform);
  return (
    <header className="topbar">
      <a className="topbar__id mono" href="#top">
        {initials(profile.name)}
        <span className="topbar__name">{profile.name}</span>
      </a>
      <nav className="topbar__nav" aria-label="Sections">
        <a href="#about">About</a>
        <a href="#work">Work</a>
        <a href="#experience">Experience</a>
        <a href="#stack">Stack</a>
        <a href="#cv">CV</a>
        <a href="#contact">Contact</a>
      </nav>
      <button type="button" className="topbar__cmd mono" onClick={onOpenPalette} aria-label="Open command menu">
        <kbd>{isMac ? '⌘' : 'Ctrl'}</kbd>
        <kbd>K</kbd>
        <span className="cmd-touch">Menu</span>
      </button>
    </header>
  );
}

export function initials(name: string) {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .map((w) => w[0]!.toUpperCase())
    .join('')
    .slice(0, 3);
}

function SectionHead({ id, label, title, children }: { id: string; label: string; title: string; children?: React.ReactNode }) {
  return (
    <header className="sec-head">
      <p className="sec-head__label mono" id={`${id}-label`}>
        {label}
      </p>
      <h2 className="sec-head__title">{title}</h2>
      {children}
    </header>
  );
}

// ── about: a model card ───────────────────────────────────────────────────────

export function ModelCard({ profile }: { profile: Profile }) {
  const slugify = (v: string) => v.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
  const slug = slugify(profile.name);
  const paragraphs = profile.bio.split(/\n\s*\n/).filter((p) => p.trim());
  return (
    <section className="section reveal" id="about" aria-labelledby="about-label">
      <SectionHead id="about" label="about" title="Model card" />
      <div className="card-grid">
        <figure className="portrait">
          {profile.avatarUrl ? (
            <img src={profile.avatarUrl} alt={`Portrait of ${profile.name}`} loading="lazy" />
          ) : (
            <div className="portrait__mono mono" aria-hidden>
              {initials(profile.name)}
            </div>
          )}
          <figcaption className="portrait__cap mono">
            {profile.available ? (
              <>
                <span className="dot dot--live" aria-hidden /> open to new roles
              </>
            ) : (
              <>
                <span className="dot" aria-hidden /> not looking right now
              </>
            )}
          </figcaption>
        </figure>

        <article className="mcard">
          <header className="mcard__head">
            <p className="mcard__repo mono">
              <span className="mcard__owner">{slug}</span> / {slugify(profile.role)}
            </p>
            {profile.focusAreas.length > 0 && (
              <ul className="tags" aria-label="Focus areas">
                {profile.focusAreas.map((f) => (
                  <li key={f}>{f}</li>
                ))}
              </ul>
            )}
          </header>

          {profile.facts.length > 0 && (
            <dl className="facts">
              {profile.facts.map((f) => (
                <div className="facts__row" key={f.label}>
                  <dt className="mono">{f.label}</dt>
                  <dd>
                    {f.url ? (
                      <a href={f.url} target="_blank" rel="noreferrer">
                        {f.value} <span aria-hidden>↗</span>
                      </a>
                    ) : (
                      f.value
                    )}
                  </dd>
                </div>
              ))}
            </dl>
          )}

          <div className="mcard__bio">
            {paragraphs.map((p, i) => (
              <p key={i}>{p}</p>
            ))}
          </div>
        </article>
      </div>
    </section>
  );
}

// ── work: an index you can filter and open ────────────────────────────────────

type Filter = 'all' | Category;

export function Work({ projects, filter, setFilter }: { projects: Project[]; filter: Filter; setFilter: (f: Filter) => void }) {
  const [open, setOpen] = useState<number | null>(null);
  const [preview, setPreview] = useState<{ src: string; x: number; y: number } | null>(null);

  const counts = useMemo(() => {
    const c: Partial<Record<Category, number>> = {};
    projects.forEach((p) => (c[p.category] = (c[p.category] ?? 0) + 1));
    return c;
  }, [projects]);
  const cats = (Object.keys(CATEGORY_LABEL) as Category[]).filter((c) => counts[c]);
  const shown = filter === 'all' ? projects : projects.filter((p) => p.category === filter);

  return (
    <section className="section reveal" id="work" aria-labelledby="work-label">
      <SectionHead id="work" label="work" title="What I've built">
        <div className="filters" role="group" aria-label="Filter by area">
          <button type="button" aria-pressed={filter === 'all'} onClick={() => setFilter('all')}>
            All <span className="mono">{projects.length}</span>
          </button>
          {cats.map((c) => (
            <button type="button" key={c} aria-pressed={filter === c} onClick={() => setFilter(c)}>
              {CATEGORY_LABEL[c]} <span className="mono">{counts[c]}</span>
            </button>
          ))}
        </div>
      </SectionHead>

      <ol className="index" onMouseLeave={() => setPreview(null)}>
        {shown.map((p) => {
          const isOpen = open === p.id;
          return (
            <li key={p.id} className={`index__item${isOpen ? ' is-open' : ''}${p.featured ? ' is-featured' : ''}`}>
              <button
                type="button"
                className="index__row"
                aria-expanded={isOpen}
                aria-controls={`project-${p.id}`}
                onClick={() => setOpen(isOpen ? null : p.id)}
                onMouseMove={(e) => p.imageUrl && !isOpen && setPreview({ src: p.imageUrl, x: e.clientX, y: e.clientY })}
                onMouseLeave={() => setPreview(null)}
              >
                <span className="index__title">
                  {p.title}
                  {p.featured && <span className="index__star mono" title="Featured">featured</span>}
                </span>
                <span className="index__plat mono">{p.platforms || CATEGORY_LABEL[p.category]}</span>
                <span className="index__summary">{p.summary}</span>
                <span className="index__toggle mono" aria-hidden>
                  {isOpen ? '−' : '+'}
                </span>
              </button>

              <div className="index__detail" id={`project-${p.id}`} hidden={!isOpen}>
                {isOpen && (
                  <div className="detail">
                    {p.imageUrl && (
                      <div className="detail__media">
                        <img src={p.imageUrl} alt={`Screenshot of ${p.title}`} loading="lazy" />
                      </div>
                    )}
                    <div className="detail__body">
                      {p.description.split(/\n\s*\n/).filter(Boolean).map((para, i) => (
                        <p key={i}>{para}</p>
                      ))}
                      {p.tech.length > 0 && (
                        <ul className="chips" aria-label="Stack">
                          {p.tech.map((t) => (
                            <li key={t}>{t}</li>
                          ))}
                        </ul>
                      )}
                      <div className="detail__links">
                        {p.liveUrl && <ExtLink href={p.liveUrl}>{linkLabel(p.liveUrl)}</ExtLink>}
                        {p.altUrl && <ExtLink href={p.altUrl}>{linkLabel(p.altUrl)}</ExtLink>}
                        {p.repoUrl && <ExtLink href={p.repoUrl}>Source code</ExtLink>}
                      </div>
                    </div>
                  </div>
                )}
              </div>
            </li>
          );
        })}
      </ol>

      {preview && (
        <img
          className="cursor-preview"
          src={preview.src}
          alt=""
          aria-hidden
          style={{ transform: `translate(${preview.x + 24}px, ${preview.y - 90}px)` }}
        />
      )}
    </section>
  );
}

function linkLabel(url: string) {
  if (url === '/') return 'You are here';
  if (url.includes('play.google.com')) return 'Google Play';
  if (url.includes('apps.apple.com')) return 'App Store';
  if (url.endsWith('.pdf')) return 'Read the paper';
  if (url.includes('github.com')) return 'Source code';
  try {
    return new URL(url).hostname.replace(/^www\./, '');
  } catch {
    return 'Open';
  }
}

function ExtLink({ href, children }: { href: string; children: React.ReactNode }) {
  const internal = href.startsWith('/');
  return (
    <a className="btn btn--small" href={href} {...(internal ? {} : { target: '_blank', rel: 'noreferrer' })}>
      {children} {!internal && <span aria-hidden>↗</span>}
    </a>
  );
}

// ── experience ────────────────────────────────────────────────────────────────

function fmtPeriod(v: string) {
  if (!v) return '';
  const [y, m] = v.split('-');
  if (!m) return y;
  const month = new Date(Number(y), Number(m) - 1).toLocaleString('en', { month: 'short' });
  return `${month} ${y}`;
}

export function Timeline({ items }: { items: Experience[] }) {
  const work = items.filter((e) => e.kind === 'work');
  const edu = items.filter((e) => e.kind === 'education');
  if (items.length === 0) return null;
  return (
    <section className="section reveal" id="experience" aria-labelledby="experience-label">
      <SectionHead id="experience" label="experience" title="Where I've worked" />
      <ol className="timeline">
        {work.map((e, i) => (
          <TimelineItem key={e.id} e={e} heat={1 - i / Math.max(work.length, 2)} />
        ))}
      </ol>
      {edu.length > 0 && (
        <>
          <h3 className="sub-title mono">education</h3>
          <ol className="timeline timeline--edu">
            {edu.map((e) => (
              <TimelineItem key={e.id} e={e} heat={0.15} />
            ))}
          </ol>
        </>
      )}
    </section>
  );
}

function TimelineItem({ e, heat }: { e: Experience; heat: number }) {
  const start = fmtPeriod(e.periodStart);
  const end = e.periodEnd ? fmtPeriod(e.periodEnd) : e.kind === 'work' ? 'Present' : '';
  return (
    <li className="tl" style={{ '--heat': magma(0.25 + heat * 0.65) } as React.CSSProperties}>
      <p className="tl__when mono">
        {start && end ? `${start} – ${end}` : start || end}
      </p>
      <div className="tl__body">
        <h3 className="tl__title">
          {e.title}
          {e.organization && <span className="tl__org"> · {e.organization}</span>}
        </h3>
        {e.summary && <p className="tl__summary">{e.summary}</p>}
        {e.highlights.length > 0 && (
          <ul className="tl__list">
            {e.highlights.map((h) => (
              <li key={h}>{h}</li>
            ))}
          </ul>
        )}
        {e.tech.length > 0 && (
          <ul className="chips chips--quiet" aria-label="Stack">
            {e.tech.map((t) => (
              <li key={t}>{t}</li>
            ))}
          </ul>
        )}
      </div>
    </li>
  );
}

// ── stack ─────────────────────────────────────────────────────────────────────

const LEVEL_RANK: Record<string, number> = { Expert: 4, Advanced: 3, Intermediate: 2, Basic: 1 };

export function Stack({ skills, projects }: { skills: Skill[]; projects: Project[] }) {
  const groups = useMemo(() => {
    const m = new Map<string, Skill[]>();
    skills.forEach((s) => m.set(s.category || 'Other', [...(m.get(s.category || 'Other') ?? []), s]));
    return [...m.entries()];
  }, [skills]);

  // How often each technology shows up across shipped projects.
  const usage = useMemo(() => {
    const m = new Map<string, number>();
    projects.forEach((p) => p.tech.forEach((t) => m.set(t, (m.get(t) ?? 0) + 1)));
    return [...m.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).slice(0, 14);
  }, [projects]);
  const top = usage[0]?.[1] ?? 1;

  if (skills.length === 0 && usage.length === 0) return null;

  return (
    <section className="section reveal" id="stack" aria-labelledby="stack-label">
      <SectionHead id="stack" label="stack" title="What I work with" />
      <div className="stack">
        <div className="stack__skills">
          {groups.map(([cat, list]) => (
            <div className="skillgroup" key={cat}>
              <h3 className="skillgroup__title mono">{cat}</h3>
              <ul>
                {list.map((s) => (
                  <li key={s.id} className="skill">
                    <span className="skill__name">{s.name}</span>
                    <span className="meter" role="img" aria-label={`${s.level}`}>
                      {SKILL_LEVELS.slice()
                        .reverse()
                        .map((lvl, i) => (
                          <i key={lvl} style={{ background: i < LEVEL_RANK[s.level] ? magma(0.3 + i * 0.2) : undefined }} />
                        ))}
                    </span>
                    <span className="skill__level mono">{s.level}</span>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>

        {usage.length > 0 && (
          <figure className="usage">
            <figcaption className="usage__cap">
              <span className="mono">frequency</span> Most-used across the {projects.length} projects above
            </figcaption>
            <ol>
              {usage.map(([name, n]) => (
                <li key={name}>
                  <span className="usage__name">{name}</span>
                  <span className="usage__bar">
                    <span style={{ width: `${(n / top) * 100}%`, background: magma(0.3 + (n / top) * 0.6) }} />
                  </span>
                  <span className="usage__n mono">{n}</span>
                </li>
              ))}
            </ol>
          </figure>
        )}
      </div>
    </section>
  );
}

// ── cv ────────────────────────────────────────────────────────────────────────

function fmtBytes(n: number) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

export function CvPanel({ profile }: { profile: Profile }) {
  const cv = profile.cv;
  return (
    <section className="section reveal" id="cv" aria-labelledby="cv-label">
      <SectionHead id="cv" label="cv" title="Take the whole thing with you" />
      {cv ? (
        <div className="ckpt">
          <div className="ckpt__file">
            <span className="ckpt__icon mono" aria-hidden>
              PDF
            </span>
            <div>
              <p className="ckpt__name mono">{cv.filename}</p>
              <p className="ckpt__meta mono">
                {fmtBytes(cv.size)}
                {cv.updatedAt && ` · updated ${new Date(cv.updatedAt).toLocaleDateString('en', { day: 'numeric', month: 'short', year: 'numeric' })}`}
                {cv.sha256 && (
                  <>
                    {' · '}
                    <span title={`sha256 ${cv.sha256}`}>sha256 {cv.sha256.slice(0, 8)}…{cv.sha256.slice(-4)}</span>
                  </>
                )}
              </p>
            </div>
          </div>
          <div className="ckpt__actions">
            <a className="btn btn--primary" href={cv.url} download>
              Download CV
            </a>
            <a className="btn" href={`${cv.url}?inline=1`} target="_blank" rel="noreferrer">
              View in browser
            </a>
          </div>
        </div>
      ) : (
        <div className="ckpt ckpt--empty">
          <p>The CV is being updated. Email me and I'll send the latest copy.</p>
          {profile.email && (
            <a className="btn btn--primary" href={`mailto:${profile.email}?subject=CV%20request`}>
              Request CV by email
            </a>
          )}
        </div>
      )}
    </section>
  );
}

// ── contact ───────────────────────────────────────────────────────────────────

type SendState = { kind: 'idle' } | { kind: 'sending' } | { kind: 'sent'; email: string } | { kind: 'error'; message: string };

export function Contact({ profile }: { profile: Profile }) {
  const [state, setState] = useState<SendState>({ kind: 'idle' });
  const [copied, setCopied] = useState(false);
  const formRef = useRef<HTMLFormElement>(null);

  useEffect(() => {
    if (!copied) return;
    const t = window.setTimeout(() => setCopied(false), 1800);
    return () => window.clearTimeout(t);
  }, [copied]);

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const input = {
      name: String(f.get('name') ?? ''),
      email: String(f.get('email') ?? ''),
      message: String(f.get('message') ?? ''),
      website: String(f.get('website') ?? ''),
    };
    setState({ kind: 'sending' });
    try {
      await publicApi.sendMessage(input);
      setState({ kind: 'sent', email: input.email });
      formRef.current?.reset();
    } catch (err) {
      setState({ kind: 'error', message: err instanceof ApiError ? err.message : 'Your message was not sent. Try again.' });
    }
  }

  return (
    <section className="section reveal" id="contact" aria-labelledby="contact-label">
      <SectionHead id="contact" label="contact" title="Send me a prompt" />
      <div className="contact">
        <form className="form" ref={formRef} onSubmit={onSubmit}>
          <div className="form__row">
            <label>
              <span>Name</span>
              <input name="name" required maxLength={120} autoComplete="name" />
            </label>
            <label>
              <span>Email</span>
              <input name="email" type="email" required autoComplete="email" />
            </label>
          </div>
          <label>
            <span>Message</span>
            <textarea name="message" required minLength={10} maxLength={5000} rows={5} placeholder="What are you building, and where could I help?" />
          </label>
          <label className="hp" aria-hidden>
            Website <input name="website" tabIndex={-1} autoComplete="off" />
          </label>
          <div className="form__foot">
            <button className="btn btn--primary" type="submit" disabled={state.kind === 'sending'}>
              {state.kind === 'sending' ? 'Sending…' : 'Send message'}
            </button>
            <p className={`form__status${state.kind === 'error' ? ' is-error' : ''}`} role="status" aria-live="polite">
              {state.kind === 'sent' && `Message sent. I'll reply to ${state.email}.`}
              {state.kind === 'error' && state.message}
            </p>
          </div>
        </form>

        <aside className="contact__side">
          {profile.email && (
            <div className="contact__email">
              <p className="mono">email</p>
              <a href={`mailto:${profile.email}`}>{profile.email}</a>
              <button
                type="button"
                className="btn btn--small"
                onClick={() => navigator.clipboard?.writeText(profile.email).then(() => setCopied(true))}
              >
                {copied ? 'Copied' : 'Copy address'}
              </button>
            </div>
          )}
          {profile.socials.length > 0 && (
            <ul className="socials">
              {profile.socials.map((s) => (
                <li key={s.url}>
                  <a href={s.url} target="_blank" rel="noreferrer">
                    {s.label} <span aria-hidden>↗</span>
                  </a>
                </li>
              ))}
            </ul>
          )}
        </aside>
      </div>
    </section>
  );
}
