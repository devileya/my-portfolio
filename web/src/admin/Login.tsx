import { useState, type FormEvent } from 'react';
import { ApiError, adminApi } from '../lib/api';
import type { Admin } from '../lib/types';

export default function Login({ notice, onSignedIn }: { notice?: string; onSignedIn: (a: Admin) => void }) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      onSignedIn(await adminApi.login(email, password));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Sign-in failed. Try again.');
      setBusy(false);
    }
  }

  return (
    <div className="adm-center">
      <form className="adm-login" onSubmit={submit} noValidate>
        <div className="adm-brand">
          <span className="adm-brand-mark" aria-hidden="true" />
          <span>dashboard</span>
        </div>
        <h1>Sign in</h1>
        {notice && !error && <p className="adm-hint">{notice}</p>}
        <div className="adm-field">
          <label htmlFor="login-email">Email</label>
          <input
            id="login-email"
            type="email"
            autoComplete="username"
            required
            autoFocus
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </div>
        <div className="adm-field">
          <label htmlFor="login-password">Password</label>
          <input
            id="login-password"
            type="password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </div>
        {error && (
          <p className="adm-error-text" role="alert">
            {error}
          </p>
        )}
        <button type="submit" className="adm-btn adm-btn--primary" disabled={busy || !email || !password}>
          {busy ? 'Signing in…' : 'Sign in'}
        </button>
      </form>
    </div>
  );
}
