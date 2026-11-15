/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_MODE?: 'real' | 'mock';
  readonly VITE_API_URL?: string;
  readonly VITE_SITES_DOMAIN?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
