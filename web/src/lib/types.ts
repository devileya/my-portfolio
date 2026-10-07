// Mirrors internal/content/content.go. Keep the two in step.

export interface Fact {
  label: string;
  value: string;
  url?: string;
}

export interface Social {
  label: string;
  url: string;
}

export interface CV {
  filename: string;
  size: number;
  sha256: string;
  updatedAt: string | null;
  url: string;
}

export interface Profile {
  name: string;
  role: string;
  headlines: string[];
  prompt: string;
  summary: string;
  bio: string;
  location: string;
  email: string;
  yearsExperience: number;
  available: boolean;
  focusAreas: string[];
  facts: Fact[];
  socials: Social[];
  avatarKey: string;
  avatarUrl: string;
  cv: CV | null;
  updatedAt: string;
}

export type Category = 'ai' | 'mobile' | 'web' | 'backend';

export const CATEGORY_LABEL: Record<Category, string> = {
  ai: 'AI / ML',
  mobile: 'Mobile',
  web: 'Web',
  backend: 'Backend',
};

export interface Project {
  id: number;
  title: string;
  category: Category;
  platforms: string;
  summary: string;
  description: string;
  tech: string[];
  imageKey: string;
  imageUrl: string;
  liveUrl: string;
  altUrl: string;
  repoUrl: string;
  featured: boolean;
  published: boolean;
  sortOrder: number;
}

export interface Experience {
  id: number;
  kind: 'work' | 'education';
  title: string;
  organization: string;
  periodStart: string; // "YYYY-MM", "YYYY" or ""
  periodEnd: string; // "" means present (for work)
  summary: string;
  highlights: string[];
  tech: string[];
  published: boolean;
  sortOrder: number;
}

export type SkillLevel = 'Expert' | 'Advanced' | 'Intermediate' | 'Basic';
export const SKILL_LEVELS: SkillLevel[] = ['Expert', 'Advanced', 'Intermediate', 'Basic'];

export interface Skill {
  id: number;
  name: string;
  category: string;
  level: SkillLevel;
  sortOrder: number;
}

export interface Message {
  id: number;
  name: string;
  email: string;
  body: string;
  read: boolean;
  createdAt: string;
}

export interface Site {
  profile: Profile;
  projects: Project[];
  experiences: Experience[];
  skills: Skill[];
}

export interface Stats {
  cvDownloads: number;
  siteViews: number;
  projects: number;
  unreadMessages: number;
  messages: number;
}

export interface UploadedObject {
  key: string;
  url: string;
  contentType: string;
  size: number;
  sha256: string;
}

export interface Admin {
  id: number;
  email: string;
}
