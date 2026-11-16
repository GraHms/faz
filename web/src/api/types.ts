// The fazd JSON contract. Field names follow internal/store, internal/product
// and internal/spec exactly; keep them in step when the Go types change.

export type ISODate = string;

export interface User {
  id: string;
  email: string;
  name: string;
  created_at: ISODate;
}

export interface Account {
  user: User;
  balance: number;
}

export interface Session {
  token: string;
  expires_at: ISODate;
  account: Account;
}

export interface Pricing {
  mzn_per_credit: number;
  signup_gift: number;
  first_page: number;
  edit: number;
  structural_edit: number;
  topup_min_mzn: number;
  topup_max_mzn: number;
}

export type LedgerKind = 'signup_gift' | 'topup' | 'charge' | 'refund' | 'adjustment';

export interface LedgerEntry {
  id: number;
  delta: number;
  kind: LedgerKind;
  reason: string;
  job_id?: number;
  payment_id?: string;
  created_at: ISODate;
}

export interface Credits {
  balance: number;
  ledger: LedgerEntry[];
  pricing: Pricing;
}

export type SiteStatus = 'questions' | 'building' | 'ready' | 'editing' | 'failed';

export interface Site {
  id: string;
  business_name: string;
  business_type: string;
  prompt: string;
  status: SiteStatus;
  /** Stored answers as labels (choices), +258 numbers (phones) or text, plus CORRECTION_KEY. Kept after a failed build. */
  answers?: Record<string, string>;
  current_version: number;
  published_version?: number;
  subdomain?: string;
  published_at?: ISODate;
  created_at: ISODate;
  updated_at: ISODate;
}

export type QuestionKind = 'choice' | 'multi' | 'phone' | 'text';

export interface QuestionOption {
  value: string;
  label: string;
}

export interface Question {
  id: string;
  text: string;
  hint: string;
  kind: QuestionKind;
  options: QuestionOption[] | null;
}

export interface Message {
  id: number;
  role: 'user' | 'assistant';
  text: string;
  /** Quick replies the assistant offers; sending one is an ordinary message. */
  options?: string[];
  job_id?: number;
  credits_charged: number;
  created_at: ISODate;
}

export interface Version {
  number: number;
  kind: 'build' | 'edit';
  status: string;
  instruction?: string;
  job_id?: number;
  created_at: ISODate;
}

export interface Asset {
  name: string;
  media_type: string;
  size: number;
  created_at: ISODate;
}

/** Key, among a site's stored answers, of the owner's correction to `understood`. */
export const CORRECTION_KEY = '_correction';

/** Photos that may come with a new site (product.MaxNewSiteFiles). */
export const MAX_NEW_SITE_FILES = 8;

export interface SiteView {
  site: Site;
  questions?: Question[];
  /** What the intake understood, shown with the questions so the owner can correct it. */
  understood?: string;
  messages: Message[];
  versions: Version[];
  assets: Asset[];
  live_url?: string;
  /** A build or edit is queued or running. */
  busy: boolean;
}

/** How the backend sorted a chat message (spec.Classification). Only edit and structural cost credits. */
export type MessageKind = 'edit' | 'structural' | 'vague' | 'other';

export interface MessageResult {
  messages: Message[];
  job_id?: number;
  credits_charged: number;
  kind: MessageKind;
}

export interface Preview {
  url: string;
  version: number;
  expires_at: ISODate;
}

export interface Published {
  url: string;
  subdomain: string;
  version: number;
}

export interface Availability {
  name: string;
  available: boolean;
  reason: string;
}

export type JobStatus = 'queued' | 'running' | 'succeeded' | 'failed';

/** A pipeline stage name, or "verify" for the rendered checks. */
export type JobStage = 'intake' | 'content' | 'plan' | 'critique' | 'build' | 'review' | 'repair' | 'edit' | 'verify' | string;

export interface Job {
  id: number;
  kind: 'build' | 'edit' | 'settle' | string;
  status: JobStatus;
  attempts: number;
  last_error?: string;
  progress?: JobStage;
  result?: unknown;
  created_at: ISODate;
  updated_at: ISODate;
}

export type PaymentStatus = 'pending' | 'succeeded' | 'failed';

export interface Payment {
  id: string;
  method: 'mpesa' | 'emola';
  msisdn: string;
  amount_mzn: number;
  vat_mzn: number;
  credits: number;
  nuit?: string;
  status: PaymentStatus;
  provider: string;
  provider_ref?: string;
  failure_reason?: string;
  created_at: ISODate;
  updated_at: ISODate;
}

export interface ApiErrorBody {
  error: { code: string; message: string; details?: Record<string, unknown> };
}

// ---- Telemetry (operators only, behind the admin key) ----
// Mirrors internal/product/telemetry.go and internal/store/usage.go. Go
// flattens embedded structs, so totals sit beside the other fields.

export type Operation = 'questions' | 'build' | 'classify' | 'edit';

export interface UsageTotals {
  calls: number;
  errors: number;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  cost_usd: number;
  latency_ms: number;
}

export interface Money {
  usd_to_mzn: number;
  cost_usd: number;
  cost_mzn: number;
  credits_charged: number;
  credits_refunded: number;
  credits_net: number;
  /** As charged, IVA included. */
  credits_mzn: number;
  /** What the business keeps; gift credits are valued at list price too. */
  credits_mzn_ex_vat: number;
  margin_mzn: number;
  margin_percent: number;
}

export interface StageTelemetry extends UsageTotals {
  operation: Operation | '';
  stage: string;
  model: string;
  effort: string;
  retries: number;
}

export interface UsageCall {
  id: number;
  job_id?: number;
  operation: Operation;
  stage: string;
  model: string;
  effort: string;
  attempt: number;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  cost_usd: number;
  latency_ms: number;
  error?: string;
  created_at: ISODate;
}

export interface SiteJobs {
  builds: number;
  edits: number;
  failed: number;
  avg_build_seconds: number;
}

export interface SiteTelemetry {
  site: Site;
  owner_email: string;
  totals: UsageTotals;
  money: Money;
  jobs: SiteJobs;
  by_operation: StageTelemetry[];
  by_model: StageTelemetry[];
  by_stage: StageTelemetry[];
  calls: UsageCall[];
}

export interface UsageGroup extends UsageTotals {
  operation: Operation;
  stage: string;
  model: string;
}

export interface OverviewSite extends UsageTotals {
  site_id: string;
  business_name: string;
  owner_email: string;
  status: SiteStatus;
  current_version: number;
  published_version?: number;
  created_at: ISODate;
  credits_charged: number;
  credits_refunded: number;
  money: Money;
}

export interface TelemetryOverview {
  from: ISODate;
  to: ISODate;
  totals: UsageTotals;
  money: Money;
  by_stage: UsageGroup[];
  sites: OverviewSite[];
}
