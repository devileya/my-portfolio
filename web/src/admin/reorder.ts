import { useState } from 'react';
import { useSession } from './session';
import { move } from './ui';

/**
 * Moves an item optimistically and persists the new order; on failure the
 * previous order comes back and the error is shown.
 */
export function useReorder<T extends { id: number }>(
  items: T[] | null,
  setItems: (v: T[]) => void,
  persist: (ids: number[]) => Promise<void>,
) {
  const { errorText, notify } = useSession();
  const [saving, setSaving] = useState(false);

  async function moveItem(from: number, to: number) {
    if (!items || saving) return;
    const before = items;
    const next = move(items, from, to);
    setItems(next);
    setSaving(true);
    try {
      await persist(next.map((x) => x.id));
    } catch (err) {
      setItems(before);
      notify(errorText(err), 'error');
    } finally {
      setSaving(false);
    }
  }

  return { moveItem, saving };
}
