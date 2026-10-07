import { StrictMode, Suspense, lazy } from 'react';
import { createRoot } from 'react-dom/client';
import '@fontsource-variable/martian-mono';
import '@fontsource-variable/schibsted-grotesk';
import './styles/tokens.css';
import { adminBase } from './lib/admin-base';
import PublicSite from './public/PublicSite';

// The dashboard is a separate chunk: visitors never download it.
const AdminApp = lazy(() => import('./admin/AdminApp'));

const base = adminBase();

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    {base ? (
      <Suspense fallback={null}>
        <AdminApp base={base} />
      </Suspense>
    ) : (
      <PublicSite />
    )}
  </StrictMode>,
);
