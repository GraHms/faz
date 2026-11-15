// What a visitor typed on the landing page, carried through sign up so they
// never type it twice. Text survives a reload (sessionStorage); attached
// photos only live in memory, because files cannot be stored there.

export interface Draft {
  prompt: string;
  businessName?: string;
  businessType?: string;
}

const KEY = 'faz.draft';
let files: File[] = [];

export const draft = {
  get(): Draft | null {
    try {
      const raw = sessionStorage.getItem(KEY);
      return raw ? (JSON.parse(raw) as Draft) : null;
    } catch {
      return null;
    }
  },
  set(d: Draft) {
    try {
      sessionStorage.setItem(KEY, JSON.stringify(d));
    } catch {
      /* the draft is a convenience */
    }
  },
  files: () => files,
  setFiles(f: File[]) {
    files = f;
  },
  clear() {
    files = [];
    try {
      sessionStorage.removeItem(KEY);
    } catch {
      /* nothing to clear */
    }
  },
};

// The backend (internal/product/images.go) reads JPEG, PNG and WebP only.
const imageTypes = ['image/jpeg', 'image/png', 'image/webp'];
export const imageAccept = imageTypes.join(',');
export const maxImageBytes = 15 * 1024 * 1024;

/** Keeps photos the backend takes; says why the others were left out. */
export function pickImages(list: FileList | File[]): { ok: File[]; rejected: string | null } {
  const ok: File[] = [];
  let rejected: string | null = null;
  for (const f of Array.from(list)) {
    if (!imageTypes.includes(f.type)) rejected = `${f.name} não é uma foto (JPG, PNG ou WebP).`;
    else if (f.size > maxImageBytes) rejected = `${f.name} tem mais de 15 MB.`;
    else ok.push(f);
  }
  return { ok, rejected };
}
