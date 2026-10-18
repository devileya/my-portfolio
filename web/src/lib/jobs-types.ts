// Mirrors internal/jobs/model.go. Keep the two in step.

export type Portal = 'linkedin' | 'indeed' | 'glints' | 'kalibrr';
export const PORTALS: Portal[] = ['linkedin', 'indeed', 'glints', 'kalibrr'];
export const PORTAL_LABEL: Record<Portal, string> = {
  linkedin: 'LinkedIn',
  indeed: 'Indeed',
  glints: 'Glints',
  kalibrr: 'Kalibrr',
};

export const WORK_MODES = ['remote', 'hybrid', 'onsite'] as const;
export const JOB_TYPES = ['full_time', 'contract', 'part_time', 'internship', 'freelance'] as const;
export const SENIORITY = ['internship', 'entry', 'associate', 'mid', 'senior', 'lead', 'director'] as const;

export const label = (v: string) => (v === 'onsite' ? 'On-site' : v.charAt(0).toUpperCase() + v.slice(1).replace(/_/g, ' '));

export interface JobSettings {
  paused: boolean;
  dryRun: boolean;
  timezone: string;
  applyStartHour: number;
  applyEndHour: number;
  minGapMinutes: number;
  maxGapMinutes: number;
  portalCaps: Partial<Record<Portal, number>>;
  scoreProviderId: number | null;
  applyProviderId: number | null;
  applicant: Record<string, string>;
  coverLetterStyle: string;
  notifyMatches: boolean;
  notifyApplied: boolean;
  notifyDigest: boolean;
  digestHour: number;
  telegram: TelegramInfo;
  workerName: string;
  workerState: string;
  workerSeenAt: string | null;
  updatedAt: string;
}

export interface TelegramInfo {
  hasToken: boolean;
  tokenHint: string;
  botName: string;
  chatId: number;
  chatName: string;
  poll: boolean;
}

export interface AIProvider {
  id: number;
  label: string;
  kind: 'anthropic' | 'openai';
  baseUrl: string;
  model: string;
  keyHint: string;
  hasKey: boolean;
  apiKey?: string;
  createdAt: string;
  updatedAt: string;
}

export type AccountStatus = 'disconnected' | 'connected' | 'expired' | 'challenge' | 'error';

export interface JobAccount {
  portal: Portal;
  label: string;
  enabled: boolean;
  status: AccountStatus;
  statusNote: string;
  cookiesCount: number;
  checkedAt: string | null;
  updatedAt: string;
}

export interface JobSearch {
  id: number;
  name: string;
  enabled: boolean;
  portals: Portal[];
  keywords: string[];
  excludeKeywords: string[];
  excludeCompanies: string[];
  locations: string[];
  workModes: string[];
  jobTypes: string[];
  seniority: string[];
  minSalary: number;
  postedWithinDays: number;
  easyApplyOnly: boolean;
  schedule: string;
  maxResults: number;
  minScore: number;
  autoApply: boolean;
  autoMinScore: number;
  dailyCap: number;
  notes: string;
  lastRunAt: string | null;
  nextRunAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export type ListingStatus =
  | 'new'
  | 'filtered'
  | 'queued'
  | 'skipped'
  | 'approved'
  | 'applying'
  | 'applied'
  | 'needs_input'
  | 'external'
  | 'failed'
  | 'dry_run';

export const STATUS_LABEL: Record<ListingStatus, string> = {
  new: 'Not scored',
  filtered: 'Filtered out',
  queued: 'To review',
  skipped: 'Skipped',
  approved: 'Approved',
  applying: 'Applying',
  applied: 'Applied',
  needs_input: 'Needs answers',
  external: 'Apply on site',
  failed: 'Failed',
  dry_run: 'Dry run',
};

export interface Listing {
  id: number;
  portal: Portal;
  externalId: string;
  searchId: number | null;
  searchName: string;
  url: string;
  title: string;
  company: string;
  location: string;
  workMode: string;
  jobType: string;
  salary: string;
  description?: string;
  easyApply: boolean | null;
  postedAt: string | null;
  score: number | null;
  scoreReason: string;
  scoreDetails: { verdict?: string; matched?: string[]; missing?: string[]; concerns?: string[] };
  status: ListingStatus;
  statusNote: string;
  attempts: number;
  approvedAt: string | null;
  appliedAt: string | null;
  foundAt: string;
  updatedAt: string;
}

export interface Question {
  key: string;
  question: string;
  kind: string;
  options?: string[];
}

export interface Application {
  id: number;
  listingId: number;
  listing?: Listing;
  status: 'running' | 'submitted' | 'dry_run' | 'needs_input' | 'external' | 'failed';
  answers: { question: string; answer: string; source: string }[];
  pendingQuestions: Question[];
  coverLetter: string;
  screenshotKey: string;
  screenshotUrl: string;
  error: string;
  steps: number;
  startedAt: string;
  finishedAt: string | null;
}

export interface SavedAnswer {
  id: number;
  key: string;
  question: string;
  answer: string;
  source: 'manual' | 'ai';
  approved: boolean;
  usedCount: number;
  updatedAt: string;
}

export interface JobTask {
  id: number;
  kind: 'search' | 'login' | 'check';
  searchId: number | null;
  searchName: string;
  portal: Portal | '';
  status: 'pending' | 'running' | 'done' | 'failed' | 'cancelled';
  trigger: string;
  runAfter: string;
  lockedBy: string;
  startedAt: string | null;
  finishedAt: string | null;
  result: { found?: number; new?: number; filtered?: number; scored?: number; queued?: number; approved?: number };
  log: string;
  error: string;
  createdAt: string;
}

export interface JobsOverview {
  settings: JobSettings;
  accounts: JobAccount[];
  byStatus: Partial<Record<ListingStatus, number>>;
  appliedToday: Partial<Record<Portal, number>>;
  appliedWeek: number;
  searches: number;
  activeSearches: number;
  pendingTasks: number;
  recentTasks: JobTask[];
  telegramReady: boolean;
  secretsReady: boolean;
  workerMode: 'embedded' | 'off';
  providers: number;
  topQueued: Listing[];
}
