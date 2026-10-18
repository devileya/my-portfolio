import { Link } from 'react-router-dom';
import { adminApi } from '../lib/api';
import { ErrorNote, Loading, PageHeader, Panel, useLoad } from './ui';
import Visitors from './Visitors';

export default function Overview() {
  const stats = useLoad(adminApi.stats);
  const health = useLoad(() => fetch('/api/health').then((r) => r.json() as Promise<{ ok: boolean; storage: boolean }>));

  return (
    <>
      <PageHeader title="Overview" hint="All-time totals since the site went live on this server, bots included. Visitors below counts people only." />

      {stats.error && <ErrorNote text={stats.error} onRetry={stats.reload} />}
      {!stats.data && !stats.error && <Loading />}
      {stats.data && (
        <div className="adm-stats">
          <Stat label="CV downloads" value={stats.data.cvDownloads} />
          <Stat label="Site views" value={stats.data.siteViews} />
          <Stat label="Published projects" value={stats.data.projects} to="/projects" />
          <Stat label="Unread messages" value={stats.data.unreadMessages} to="/messages" hot={stats.data.unreadMessages > 0} />
        </div>
      )}

      {health.data && !health.data.storage && (
        <div className="adm-warn" role="note">
          <strong>File storage is off.</strong> Image and CV uploads will fail until S3_ENDPOINT, S3_ACCESS_KEY and S3_SECRET_KEY
          point at a Garage key with access to the bucket. Text edits still work.
        </div>
      )}

      <Visitors />

      <Panel title="Quick links">
        <ul className="adm-links">
          <li>
            <Link to="/profile">Edit headline, bio and photo</Link>
          </li>
          <li>
            <Link to="/cv">Replace the downloadable CV</Link>
          </li>
          <li>
            <Link to="/projects/new">Add a project</Link>
          </li>
          <li>
            <Link to="/experience/new">Add a role or degree</Link>
          </li>
          <li>
            <a href="/" target="_blank" rel="noopener">
              Open the public site ↗
            </a>
          </li>
        </ul>
      </Panel>
    </>
  );
}

function Stat({ label, value, to, hot }: { label: string; value: number; to?: string; hot?: boolean }) {
  const body = (
    <>
      <span className="adm-stat-value">{value.toLocaleString()}</span>
      <span className="adm-stat-label">{label}</span>
    </>
  );
  return to ? (
    <Link to={to} className={`adm-stat${hot ? ' is-hot' : ''}`}>
      {body}
    </Link>
  ) : (
    <div className="adm-stat">{body}</div>
  );
}
