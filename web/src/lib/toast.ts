import { writable } from "svelte/store";

export interface Toast {
  id: number;
  message: string;
  kind: "success" | "error";
}

export const toasts = writable<Toast[]>([]);

let next = 1;

export function toast(message: string, kind: Toast["kind"] = "success") {
  const id = next++;
  toasts.update((all) => [...all, { id, message, kind }]);
  // Errors stay until dismissed; they usually need reading
  if (kind === "success") setTimeout(() => dismiss(id), 5000);
}

export function dismiss(id: number) {
  toasts.update((all) => all.filter((t) => t.id !== id));
}
