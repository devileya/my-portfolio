import { useCallback, useEffect, useMemo, useState } from 'react';
import { publicApi, ApiError } from '../lib/api';
import type { Category, Site } from '../lib/types';
import { CATEGORY_LABEL } from '../lib/types';
import CommandPalette, { type Command } from './CommandPalette';
import Hero from './Hero';
import { Contact, CvPanel, ModelCard, Stack, Timeline, TopBar, Work } from './sections';
import './site.css';

type Load = { state: 'loading' } | { state: 'ready'; site: Site } | { state: 'error'; message: string };

export default function PublicSite() {
  const [load, setLoad] = useState<Load>({ state: 'loading' });

  const fetchSite = useCallback(() => {
    setLoad({ state: 'loading' });
    publicApi
      .site()
      .then((site) => setLoad({ state: 'ready', site }))
      .catch((e) => setLoad({ state: 'error', message: e instanceof ApiError ? e.message : String(e) }));
  }, []);

  useEffect(fetchSite, [fetchSite]);

  if (location.pathname !== '/') return <NotFound />;
  if (load.state === 'loading') return <Boot />;
  if (load.state === 'error')
    return (
      <main className="fault">
        <p className="mono">error: site content unavailable</p>
        <p>{load.message}</p>
        <button className="btn" type="button" onClick={fetchSite}>
          Try again
        </button>
      </main>
    );
  return <Page site={load.site} />;
}

function Page({ site }: { site: Site }) {
  const { profile, projects, experiences, skills } = site;
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [regen, setRegen] = useState(0);
  const [filter, setFilter] = useState<'all' | Category>('all');

  useReveal();

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setPaletteOpen((o) => !o);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  const commands = useMemo<Command[]>(() => {
    const go = (id: string) => () => document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
    const cats = [...new Set(projects.map((p) => p.category))];
    const list: Command[] = [
      { id: 'about', label: 'Go to About', hint: 'model card', run: go('about') },
      { id: 'work', label: 'Go to Work', hint: `${projects.length} projects`, run: go('work') },
      ...cats.map((c) => ({
        id: `filter-${c}`,
        label: `Show ${CATEGORY_LABEL[c]} projects`,
        hint: 'filter',
        run: () => {
          setFilter(c);
          go('work')();
        },
      })),
      { id: 'experience', label: 'Go to Experience', run: go('experience') },
      { id: 'stack', label: 'Go to Stack', run: go('stack') },
      { id: 'contact', label: 'Send a message', run: go('contact') },
      { id: 'regen', label: 'Regenerate the headline', hint: 'hero', run: () => (go('top')(), setRegen((r) => r + 1)) },
    ];
    if (profile.cv) list.splice(0, 0, { id: 'cv', label: 'Download CV', hint: 'PDF', run: () => (window.location.href = '/api/cv') });
    else list.push({ id: 'cv', label: 'Go to CV', run: go('cv') });
    if (profile.email) {
      list.push({ id: 'email', label: 'Copy email address', hint: profile.email, run: () => void navigator.clipboard?.writeText(profile.email) });
    }
    profile.socials.forEach((s) =>
      list.push({ id: `social-${s.label}`, label: `Open ${s.label}`, hint: 'new tab', run: () => void window.open(s.url, '_blank', 'noopener') }),
    );
    return list;
  }, [projects, profile]);

  return (
    <>
      <a className="skip mono" href="#main">
        Skip to content
      </a>
      <TopBar profile={profile} onOpenPalette={() => setPaletteOpen(true)} />
      <main id="main">
        <Hero profile={profile} regenerateSignal={regen} />
        <ModelCard profile={profile} />
        <Work projects={projects} filter={filter} setFilter={setFilter} />
        <Timeline items={experiences} />
        <Stack skills={skills} projects={projects} />
        <CvPanel profile={profile} />
        <Contact profile={profile} />
      </main>
      <footer className="footer mono">
        <span>
          © {new Date().getFullYear()} {profile.name}
        </span>
        <span>Go · React · Postgres · Garage</span>
        <button type="button" onClick={() => setPaletteOpen(true)}>
          Command menu
        </button>
      </footer>
      <CommandPalette open={paletteOpen} onClose={() => setPaletteOpen(false)} commands={commands} />
    </>
  );
}

/** Adds .is-in to .reveal sections the first time they enter the viewport. */
function useReveal() {
  useEffect(() => {
    const els = document.querySelectorAll('.reveal');
    // A background tab never paints the transition, so don't hide anything.
    if (!('IntersectionObserver' in window) || document.visibilityState === 'hidden') {
      els.forEach((el) => el.classList.add('is-in'));
      return;
    }
    const io = new IntersectionObserver(
      (entries) =>
        entries.forEach((e) => {
          if (e.isIntersecting) {
            e.target.classList.add('is-in');
            io.unobserve(e.target);
          }
        }),
      { rootMargin: '0px 0px -10% 0px' },
    );
    els.forEach((el) => io.observe(el));
    return () => io.disconnect();
  }, []);
}

function Boot() {
  return (
    <main className="boot mono" aria-busy="true">
      <span>loading weights</span>
      <span className="boot__bar" aria-hidden>
        <span />
      </span>
    </main>
  );
}

function NotFound() {
  return (
    <main className="fault">
      <p className="mono">404 · token not in vocabulary</p>
      <p>There's nothing at {location.pathname}.</p>
      <a className="btn btn--primary" href="/">
        Back to the portfolio
      </a>
    </main>
  );
}
