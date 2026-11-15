import { useQuery } from '@tanstack/react-query';
import { api, type Job, type Payment, type SiteView } from '../api';

export const keys = {
  me: ['me'] as const,
  pricing: ['pricing'] as const,
  credits: ['credits'] as const,
  sites: ['sites'] as const,
  site: (id: string) => ['site', id] as const,
  job: (id: number) => ['job', id] as const,
  payments: ['payments'] as const,
  payment: (id: string) => ['payment', id] as const,
  preview: (id: string, version: number) => ['preview', id, version] as const,
};

export function usePricing() {
  return useQuery({ queryKey: keys.pricing, queryFn: api.pricing, staleTime: 5 * 60_000 });
}

export function useSites() {
  return useQuery({ queryKey: keys.sites, queryFn: api.sites });
}

export function useSite(id: string) {
  return useQuery({
    queryKey: keys.site(id),
    queryFn: () => api.site(id),
    // While a job runs, the job poll drives updates; this is a slow safety net.
    refetchInterval: (q) => (q.state.data?.busy ? 10_000 : false),
  });
}

export function useCredits() {
  return useQuery({ queryKey: keys.credits, queryFn: api.credits });
}

export function usePayments() {
  return useQuery({ queryKey: keys.payments, queryFn: api.payments });
}

const jobDone = (j?: Job) => j?.status === 'succeeded' || j?.status === 'failed';

/** Polls a job until it finishes. */
export function useJob(id: number | undefined) {
  return useQuery({
    queryKey: keys.job(id ?? 0),
    queryFn: () => api.job(id!),
    enabled: !!id,
    refetchInterval: (q) => (jobDone(q.state.data) ? false : 1500),
  });
}

/** Polls a payment until the wallet confirms or refuses it. */
export function usePayment(id: string | undefined) {
  return useQuery({
    queryKey: keys.payment(id ?? ''),
    queryFn: () => api.payment(id!),
    enabled: !!id,
    refetchInterval: (q) => ((q.state.data as Payment | undefined)?.status === 'pending' || !q.state.data ? 2000 : false),
  });
}

/** The job the site is waiting on: the newest message that points at one. */
export function activeJobId(v: SiteView | undefined): number | undefined {
  if (!v?.busy) return undefined;
  for (let i = v.messages.length - 1; i >= 0; i--) {
    const id = v.messages[i]!.job_id;
    if (id) return id;
  }
  return undefined;
}
