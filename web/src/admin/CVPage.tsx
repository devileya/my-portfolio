import { useRef, useState } from 'react';
import { adminApi } from '../lib/api';
import { useSession } from './session';
import { ConfirmButton, ErrorNote, Loading, PageHeader, Panel, formatBytes, formatDate, useLoad } from './ui';

export default function CVPage() {
  const { errorText, notify } = useSession();
  const { data: profile, setData, error, reload } = useLoad(adminApi.profile);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState('');
  const input = useRef<HTMLInputElement>(null);

  async function upload(file: File | undefined) {
    if (!file) return;
    setBusy(true);
    setActionError('');
    try {
      setData(await adminApi.uploadCV(file));
      notify('CV uploaded');
    } catch (err) {
      setActionError(errorText(err));
    } finally {
      setBusy(false);
      if (input.current) input.current.value = '';
    }
  }

  async function remove() {
    setBusy(true);
    setActionError('');
    try {
      setData(await adminApi.deleteCV());
      notify('CV removed');
    } catch (err) {
      setActionError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  if (error && !profile) return <ErrorNote text={error} onRetry={reload} />;
  if (!profile) return <Loading />;
  const cv = profile.cv;

  return (
    <>
      <PageHeader title="CV" hint="Visitors download this file from the CV section. Uploading replaces it everywhere at once." />
      {actionError && <ErrorNote text={actionError} />}

      <Panel title="Current file">
        {cv ? (
          <dl className="adm-meta">
            <dt>File name</dt>
            <dd>{cv.filename}</dd>
            <dt>Size</dt>
            <dd>{formatBytes(cv.size)}</dd>
            <dt>SHA-256</dt>
            <dd className="mono" title={cv.sha256}>
              {cv.sha256.slice(0, 16)}…
            </dd>
            <dt>Updated</dt>
            <dd>{formatDate(cv.updatedAt)}</dd>
          </dl>
        ) : (
          <p className="adm-muted">No CV uploaded yet. Upload a PDF and the download button appears on the site.</p>
        )}

        <div className="adm-actions">
          <input
            ref={input}
            type="file"
            accept="application/pdf"
            className="sr-only"
            id="cv-file"
            onChange={(e) => upload(e.target.files?.[0])}
          />
          <button type="button" className="adm-btn adm-btn--primary" disabled={busy} onClick={() => input.current?.click()}>
            {busy ? 'Working…' : cv ? 'Upload new CV' : 'Upload CV'}
          </button>
          {cv && (
            <>
              <a className="adm-btn adm-btn--ghost" href="/api/cv?inline=1" target="_blank" rel="noopener">
                View current CV ↗
              </a>
              <ConfirmButton label="Remove CV" confirmLabel="Yes, remove CV" onConfirm={remove} busy={busy} />
            </>
          )}
        </div>
        <p className="adm-hint">PDF only, up to 20 MB. Viewing here doesn’t count as a download.</p>
      </Panel>
    </>
  );
}
