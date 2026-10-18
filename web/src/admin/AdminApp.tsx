import { useCallback, useEffect, useState } from 'react';
import { BrowserRouter, NavLink, Navigate, Route, Routes, useLocation } from 'react-router-dom';
import { ApiError, adminApi } from '../lib/api';
import type { Admin } from '../lib/types';
import './admin.css';
import { SessionProvider, useSession } from './session';
import Login from './Login';
import Overview from './Overview';
import ProfilePage from './ProfilePage';
import CVPage from './CVPage';
import ProjectsPage, { ProjectEditor } from './ProjectsPage';
import ExperiencePage, { ExperienceEditor } from './ExperiencePage';
import SkillsPage from './SkillsPage';
import MessagesPage from './MessagesPage';
import SettingsPage from './SettingsPage';
import JobsOverview from './jobs/JobsOverview';
import QueuePage from './jobs/QueuePage';
import ListingPage from './jobs/ListingPage';
import SearchesPage, { SearchEditor } from './jobs/SearchesPage';
import AccountsPage from './jobs/AccountsPage';
import AnswersPage from './jobs/AnswersPage';
import { ActivityPage, ApplicationsPage } from './jobs/ActivityPages';
import JobSettingsPage from './jobs/JobSettingsPage';

type Boot = { state: 'loading' } | { state: 'signed-out'; notice?: string } | { state: 'in'; admin: Admin } | { state: 'error'; text: string };

export default function AdminApp({ base }: { base: string }) {
  const [boot, setBoot] = useState<Boot>({ state: 'loading' });

  const check = useCallback(() => {
    setBoot({ state: 'loading' });
    adminApi
      .me()
      .then((admin) => setBoot({ state: 'in', admin }))
      .catch((err) => {
        if (err instanceof ApiError && err.status === 401) setBoot({ state: 'signed-out' });
        else setBoot({ state: 'error', text: err instanceof Error ? err.message : 'The dashboard could not load.' });
      });
  }, []);

  useEffect(check, [check]);

  const expired = useCallback(() => setBoot({ state: 'signed-out', notice: 'Your session ended. Sign in again.' }), []);
  const signOut = useCallback(() => {
    adminApi.logout().finally(() => setBoot({ state: 'signed-out', notice: 'Signed out.' }));
  }, []);
  const loadUnread = useCallback(() => adminApi.stats().then((s) => s.unreadMessages), []);

  if (boot.state === 'loading') {
    return (
      <div className="adm-center">
        <p className="adm-muted">Checking session…</p>
      </div>
    );
  }
  if (boot.state === 'error') {
    return (
      <div className="adm-center">
        <div className="adm-error" role="alert">
          <span>{boot.text}</span>
          <button type="button" className="adm-btn adm-btn--ghost" onClick={check}>
            Try again
          </button>
        </div>
      </div>
    );
  }
  if (boot.state === 'signed-out') {
    return <Login notice={boot.notice} onSignedIn={(admin) => setBoot({ state: 'in', admin })} />;
  }

  return (
    <BrowserRouter basename={base}>
      <SessionProvider admin={boot.admin} onExpired={expired} onSignOut={signOut} loadUnread={loadUnread}>
        <Shell />
      </SessionProvider>
    </BrowserRouter>
  );
}

const NAV = [
  { to: '/', label: 'Overview', end: true },
  { to: '/profile', label: 'Profile' },
  { to: '/cv', label: 'CV' },
  { to: '/projects', label: 'Projects' },
  { to: '/experience', label: 'Experience' },
  { to: '/skills', label: 'Skills' },
  { to: '/messages', label: 'Messages' },
  { to: '/settings', label: 'Settings' },
];

const JOBS_NAV = [
  { to: '/jobs', label: 'Overview', end: true },
  { to: '/jobs/queue', label: 'Review queue' },
  { to: '/jobs/searches', label: 'Searches' },
  { to: '/jobs/applications', label: 'Applications' },
  { to: '/jobs/accounts', label: 'Accounts' },
  { to: '/jobs/answers', label: 'Answer bank' },
  { to: '/jobs/activity', label: 'Activity' },
  { to: '/jobs/settings', label: 'Settings' },
];

function Shell() {
  const { admin, unread } = useSession();
  const [menuOpen, setMenuOpen] = useState(false);
  const location = useLocation();

  useEffect(() => setMenuOpen(false), [location.pathname]);

  return (
    <div className="adm">
      <aside className={`adm-side${menuOpen ? ' is-open' : ''}`}>
        <div className="adm-brand">
          <span className="adm-brand-mark" aria-hidden="true" />
          <span>dashboard</span>
          <button
            type="button"
            className="adm-menu-btn"
            aria-expanded={menuOpen}
            aria-controls="adm-nav"
            onClick={() => setMenuOpen((o) => !o)}
          >
            {menuOpen ? 'Close' : 'Menu'}
          </button>
        </div>
        <nav id="adm-nav" className="adm-nav" aria-label="Dashboard">
          {NAV.map((n) => (
            <NavLink key={n.to} to={n.to} end={n.end}>
              <span>{n.label}</span>
              {n.to === '/messages' && unread > 0 && (
                <span className="adm-count" aria-label={`${unread} unread`}>
                  {unread}
                </span>
              )}
            </NavLink>
          ))}
          <span className="adm-nav-group">Job Hunter</span>
          {JOBS_NAV.map((n) => (
            <NavLink key={n.to} to={n.to} end={n.end}>
              <span>{n.label}</span>
            </NavLink>
          ))}
          <a href="/" target="_blank" rel="noopener" className="adm-nav-out">
            View site ↗
          </a>
        </nav>
        <p className="adm-side-foot">{admin.email}</p>
      </aside>
      <main className="adm-main">
        <Routes>
          <Route path="/" element={<Overview />} />
          <Route path="/profile" element={<ProfilePage />} />
          <Route path="/cv" element={<CVPage />} />
          <Route path="/projects" element={<ProjectsPage />} />
          <Route path="/projects/new" element={<ProjectEditor />} />
          <Route path="/projects/:id" element={<ProjectEditor />} />
          <Route path="/experience" element={<ExperiencePage />} />
          <Route path="/experience/new" element={<ExperienceEditor />} />
          <Route path="/experience/:id" element={<ExperienceEditor />} />
          <Route path="/skills" element={<SkillsPage />} />
          <Route path="/messages" element={<MessagesPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="/jobs" element={<JobsOverview />} />
          <Route path="/jobs/queue" element={<QueuePage />} />
          <Route path="/jobs/listings/:id" element={<ListingPage />} />
          <Route path="/jobs/searches" element={<SearchesPage />} />
          <Route path="/jobs/searches/new" element={<SearchEditor />} />
          <Route path="/jobs/searches/:id" element={<SearchEditor />} />
          <Route path="/jobs/applications" element={<ApplicationsPage />} />
          <Route path="/jobs/accounts" element={<AccountsPage />} />
          <Route path="/jobs/answers" element={<AnswersPage />} />
          <Route path="/jobs/activity" element={<ActivityPage />} />
          <Route path="/jobs/settings" element={<JobSettingsPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
    </div>
  );
}
