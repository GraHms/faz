import { lazy, Suspense, type ReactNode } from 'react';
import { BrowserRouter, Route, Routes, useLocation } from 'react-router-dom';
import { useEffect } from 'react';
import { RequireAuth } from './auth/session';
import { Spinner } from './components/ui';
import Landing from './pages/Landing';

// The marketing page loads first; app screens load when visited.
const Pricing = lazy(() => import('./pages/Pricing'));
const Domains = lazy(() => import('./pages/Domains'));
const Signup = lazy(() => import('./pages/Signup'));
const Login = lazy(() => import('./pages/Login'));
const NewSite = lazy(() => import('./pages/NewSite'));
const Dashboard = lazy(() => import('./pages/Dashboard'));
const Editor = lazy(() => import('./pages/editor/Editor'));
const Checkout = lazy(() => import('./pages/Checkout'));
const NotFound = lazy(() => import('./pages/NotFound'));

function Loading() {
  return (
    <div style={{ minHeight: '60vh', display: 'grid', placeItems: 'center' }}>
      <Spinner label="A carregar" />
    </div>
  );
}

/** Scrolls to the top on navigation, or to the #hash when there is one. */
function ScrollManager() {
  const { pathname, hash } = useLocation();
  useEffect(() => {
    if (hash) {
      const el = document.getElementById(hash.slice(1));
      if (el) {
        el.scrollIntoView();
        return;
      }
    }
    window.scrollTo(0, 0);
  }, [pathname, hash]);
  return null;
}

const authed = (el: ReactNode) => <RequireAuth>{el}</RequireAuth>;

export function AppRoutes() {
  return (
    <Suspense fallback={<Loading />}>
      <ScrollManager />
      <Routes>
        <Route path="/" element={<Landing />} />
        <Route path="/precos" element={<Pricing />} />
        <Route path="/dominios" element={<Domains />} />
        <Route path="/criar-conta" element={<Signup />} />
        <Route path="/entrar" element={<Login />} />
        <Route path="/novo" element={authed(<NewSite />)} />
        <Route path="/painel" element={authed(<Dashboard />)} />
        <Route path="/sites/:id" element={authed(<Editor />)} />
        <Route path="/recarregar" element={authed(<Checkout />)} />
        <Route path="*" element={<NotFound />} />
      </Routes>
    </Suspense>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <AppRoutes />
    </BrowserRouter>
  );
}
