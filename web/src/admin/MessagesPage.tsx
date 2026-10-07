import { useState } from 'react';
import { adminApi } from '../lib/api';
import type { Message } from '../lib/types';
import { useSession } from './session';
import { ConfirmButton, ErrorNote, Loading, PageHeader, Panel, formatDate, useLoad } from './ui';

export default function MessagesPage() {
  const { errorText, notify, refreshUnread } = useSession();
  const { data, setData, error, reload } = useLoad(adminApi.messages);
  const [busy, setBusy] = useState<number | null>(null);

  const sorted = data ? [...data].sort((a, b) => Number(a.read) - Number(b.read)) : null;

  async function mark(m: Message, read: boolean) {
    setBusy(m.id);
    try {
      await adminApi.markMessage(m.id, read);
      setData((data ?? []).map((x) => (x.id === m.id ? { ...x, read } : x)));
      refreshUnread();
    } catch (err) {
      notify(errorText(err), 'error');
    } finally {
      setBusy(null);
    }
  }

  async function remove(m: Message) {
    setBusy(m.id);
    try {
      await adminApi.deleteMessage(m.id);
      setData((data ?? []).filter((x) => x.id !== m.id));
      refreshUnread();
      notify('Message deleted');
    } catch (err) {
      notify(errorText(err), 'error');
    } finally {
      setBusy(null);
    }
  }

  const unread = data?.filter((m) => !m.read).length ?? 0;

  return (
    <>
      <PageHeader
        title="Messages"
        hint={data ? (unread ? `${unread} unread of ${data.length}.` : `${data.length} messages, all read.`) : undefined}
      />
      {error && <ErrorNote text={error} onRetry={reload} />}
      {!data && !error && <Loading />}
      {sorted && sorted.length === 0 && (
        <Panel>
          <p className="adm-muted">No messages yet. Anything sent through the contact form on the site lands here.</p>
        </Panel>
      )}
      {sorted && sorted.length > 0 && (
        <ul className="adm-inbox">
          {sorted.map((m) => (
            <li key={m.id} className={m.read ? 'is-read' : 'is-unread'}>
              <header>
                <div>
                  <strong>{m.name}</strong> <span className="adm-muted">&lt;{m.email}&gt;</span>
                </div>
                <time className="adm-muted mono" dateTime={m.createdAt}>
                  {formatDate(m.createdAt)}
                </time>
              </header>
              <p className="adm-inbox-body">{m.body}</p>
              <div className="adm-actions">
                <a
                  className="adm-btn adm-btn--ghost"
                  href={`mailto:${encodeURIComponent(m.email)}?subject=${encodeURIComponent('Re: your message')}`}
                  onClick={() => !m.read && mark(m, true)}
                >
                  Reply by email
                </a>
                <button type="button" className="adm-btn adm-btn--ghost" disabled={busy === m.id} onClick={() => mark(m, !m.read)}>
                  {m.read ? 'Mark unread' : 'Mark read'}
                </button>
                <ConfirmButton label="Delete" confirmLabel="Delete message" busy={busy === m.id} onConfirm={() => remove(m)} />
              </div>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
