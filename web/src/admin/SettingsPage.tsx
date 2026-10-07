import { useState, type FormEvent } from 'react';
import { adminApi } from '../lib/api';
import { useSession } from './session';
import { ErrorNote, PageHeader, Panel, TextField } from './ui';

export default function SettingsPage() {
  const { admin, errorText, notify, signOut } = useSession();
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirm, setConfirm] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const problem =
    next && next.length < 10
      ? 'Use at least 10 characters.'
      : confirm && confirm !== next
        ? 'The two new passwords don’t match.'
        : '';

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (problem) return;
    setBusy(true);
    setError('');
    try {
      await adminApi.changePassword(current, next);
      setCurrent('');
      setNext('');
      setConfirm('');
      notify('Password changed');
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <PageHeader title="Settings" hint={`Signed in as ${admin.email}.`} />

      <Panel title="Change password">
        <form className="adm-narrow" onSubmit={submit}>
          {error && <ErrorNote text={error} />}
          <TextField label="Current password" type="password" value={current} onChange={setCurrent} required />
          <TextField label="New password" type="password" value={next} onChange={setNext} hint="At least 10 characters." required />
          <TextField label="Confirm new password" type="password" value={confirm} onChange={setConfirm} required />
          {problem && <p className="adm-error-text">{problem}</p>}
          <button
            type="submit"
            className="adm-btn adm-btn--primary"
            disabled={busy || !current || !next || next !== confirm || !!problem}
          >
            {busy ? 'Changing…' : 'Change password'}
          </button>
        </form>
      </Panel>

      <Panel title="Session">
        <p className="adm-muted">Signing out clears the session cookie on this browser.</p>
        <button type="button" className="adm-btn adm-btn--ghost" onClick={signOut}>
          Sign out
        </button>
      </Panel>
    </>
  );
}
