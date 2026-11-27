import type { UpdateInfo } from "./api";

type Git = NonNullable<UpdateInfo["git"]>;

// How a ref reads in a sentence: "main", "tag 24.1.0", "the default branch"
export function refName(git: Git): string {
  if (git.ref_kind === "default") return "the default branch";
  if (git.ref_kind === "tag") return `tag ${git.ref}`;
  return git.ref ?? "";
}

export const shortCommit = (c?: string) => (c ?? "").slice(0, 7);

// sha256:4c0f…  → the first 12 hex digits, as docker prints image IDs
export const shortDigest = (d?: string) => (d ?? "").replace(/^sha256:/, "").slice(0, 12);

export function newImages(u: UpdateInfo) {
  return (u.images ?? []).filter((i) => i.update);
}

// What an update brings, in a few words, e.g. "3 new commits on main"
export function updateSummary(u: UpdateInfo): string {
  const parts: string[] = [];
  const git = u.git;
  if (git?.update) {
    if (git.ref_kind === "tag") parts.push(`${refName(git)} now points to a newer commit`);
    else if (git.behind) parts.push(`${git.behind} new ${git.behind === 1 ? "commit" : "commits"} on ${refName(git)}`);
    else parts.push(`a newer commit on ${refName(git)}`);
  }
  const images = newImages(u);
  if (images.length === 1) parts.push(`a newer ${images[0].image}`);
  else if (images.length > 1) parts.push(`newer images for ${images.length} services`);
  const text = parts.join(" and ");
  return text.charAt(0).toUpperCase() + text.slice(1);
}
