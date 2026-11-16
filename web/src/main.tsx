import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import '@fontsource-variable/geist';
import '@fontsource/geist-mono/400.css';
import '@fontsource/geist-mono/500.css';
import '@fontsource/instrument-serif/400.css';
import '@fontsource/instrument-serif/400-italic.css';
import './styles/tokens.css';
import './styles/base.css';
import App from './App';
import { SessionProvider } from './auth/session';
import { isApiError } from './api';
import { invite } from './lib/draft';

// An invite link may be the first page loaded: keep its code before any
// screen reads it.
invite.capture(window.location.search);

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // Retry network hiccups, not answers the server meant.
      retry: (n, e) => n < 2 && (!isApiError(e) || e.status === 0 || e.status >= 500),
      refetchOnWindowFocus: true,
      staleTime: 5_000,
    },
  },
});

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <SessionProvider>
        <App />
      </SessionProvider>
    </QueryClientProvider>
  </StrictMode>,
);
