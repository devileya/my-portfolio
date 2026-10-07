import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react';
import { ApiError } from '../lib/api';
import type { Admin } from '../lib/types';

interface Toast {
  id: number;
  text: string;
  tone: 'ok' | 'error';
}

interface SessionValue {
  admin: Admin;
  signOut: () => void;
  /** Turns any thrown value into a sentence; a 401 also ends the session. */
  errorText: (err: unknown) => string;
  notify: (text: string, tone?: Toast['tone']) => void;
  unread: number;
  refreshUnread: () => void;
}

const SessionContext = createContext<SessionValue | null>(null);

export function useSession(): SessionValue {
  const v = useContext(SessionContext);
  if (!v) throw new Error('useSession outside SessionProvider');
  return v;
}

export function SessionProvider({
  admin,
  onExpired,
  onSignOut,
  loadUnread,
  children,
}: {
  admin: Admin;
  onExpired: () => void;
  onSignOut: () => void;
  loadUnread: () => Promise<number>;
  children: ReactNode;
}) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const [unread, setUnread] = useState(0);
  const nextId = useRef(1);

  const notify = useCallback((text: string, tone: Toast['tone'] = 'ok') => {
    const id = nextId.current++;
    setToasts((t) => [...t, { id, text, tone }]);
    window.setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), tone === 'error' ? 7000 : 3500);
  }, []);

  const errorText = useCallback(
    (err: unknown) => {
      if (err instanceof ApiError) {
        if (err.status === 401) onExpired();
        return err.message;
      }
      return err instanceof Error ? err.message : 'Something went wrong.';
    },
    [onExpired],
  );

  const refreshUnread = useCallback(() => {
    loadUnread()
      .then(setUnread)
      .catch((err) => {
        if (err instanceof ApiError && err.status === 401) onExpired();
      });
  }, [loadUnread, onExpired]);

  useEffect(refreshUnread, [refreshUnread]);

  return (
    <SessionContext.Provider value={{ admin, signOut: onSignOut, errorText, notify, unread, refreshUnread }}>
      {children}
      <div className="adm-toasts" role="status" aria-live="polite">
        {toasts.map((t) => (
          <div key={t.id} className={`adm-toast adm-toast--${t.tone}`}>
            {t.text}
          </div>
        ))}
      </div>
    </SessionContext.Provider>
  );
}
