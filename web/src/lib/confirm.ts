import { writable } from "svelte/store";

export interface ConfirmRequest {
  title: string;
  message: string;
  confirmLabel: string;
  danger?: boolean;
  resolve: (ok: boolean) => void;
}

export const pending = writable<ConfirmRequest | null>(null);

// Asks for confirmation in a dialog; resolves to false when dismissed.
export function confirmAction(opts: Omit<ConfirmRequest, "resolve">): Promise<boolean> {
  return new Promise((resolve) => {
    pending.set({
      ...opts,
      resolve: (ok) => {
        pending.set(null);
        resolve(ok);
      },
    });
  });
}
