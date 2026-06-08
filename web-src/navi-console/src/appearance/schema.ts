import { z } from 'zod';

const ThemeTokensSchema = z.object({
  background: z.string().regex(/^#[0-9a-fA-F]{6}$/),
  surface: z.string().regex(/^#[0-9a-fA-F]{6}$/),
  text: z.string().regex(/^#[0-9a-fA-F]{6}$/),
  muted: z.string().regex(/^#[0-9a-fA-F]{6}$/),
  border: z.string().regex(/^#[0-9a-fA-F]{6}$/),
  danger: z.string().regex(/^#[0-9a-fA-F]{6}$/),
  warning: z.string().regex(/^#[0-9a-fA-F]{6}$/),
  success: z.string().regex(/^#[0-9a-fA-F]{6}$/),
});

const ThemeTokenGroupsSchema = z.object({
  light: ThemeTokensSchema,
  dark: ThemeTokensSchema,
});

export const ConsoleAppearancePatchSchema = z.object({
  mode: z.enum(['light', 'dark', 'system']).optional(),
  accent: z.string().regex(/^#[0-9a-fA-F]{6}$/).optional(),
  density: z.enum(['compact', 'comfortable']).optional(),
  font_scale: z.enum(['small', 'default', 'large']).optional(),
  tokens: ThemeTokenGroupsSchema.optional(),
});

export const ThemeProposalSchema = z.object({
  type: z.literal("theme-proposal"),
  version: z.literal(1),
  rationale: z.string().max(500),
  patch: ConsoleAppearancePatchSchema,
});

export type ThemeProposal = z.infer<typeof ThemeProposalSchema>;
export type ConsoleAppearancePatch = z.infer<typeof ConsoleAppearancePatchSchema>;
