import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { z } from 'zod';
import { naviFetch } from './client';

export const CEREMONY_QUERY_KEY = ['ceremony'] as const;

const CeremonyJourneyStatusSchema = z.enum(['not_started', 'in_progress', 'completed', 'skipped']);

const CeremonyJourneyStateSchema = z.object({
  journeyId: z.string(),
  status: CeremonyJourneyStatusSchema,
  currentStep: z.string().optional(),
  completedSteps: z.array(z.string()),
  startedAt: z.string().optional(),
  completedAt: z.string().optional(),
  skippedAt: z.string().optional(),
  version: z.string(),
});

const CeremonyOwnerProfileSeedSchema = z.object({
  displayName: z.string(),
  createdFrom: z.string().optional(),
  ownerSet: z.boolean().optional(),
});

const CeremonyPresencePreferenceSchema = z.object({
  mode: z.enum(['calm_quiet', 'warm_conversational', 'direct_strategic', 'fast_focused', 'balanced']),
  createdFrom: z.string().optional(),
  ownerSet: z.boolean().optional(),
});

const CeremonyTrustBoundariesSchema = z.object({
  confirm_before_sending_messages: z.boolean().optional(),
  confirm_before_changing_files: z.boolean().optional(),
  confirm_before_purchases: z.boolean().optional(),
  confirm_before_remembering_sensitive_details: z.boolean().optional(),
  confirm_before_acting_on_inferred_preferences: z.boolean().optional(),
  confirm_before_interrupting_proactively: z.boolean().optional(),
  confirm_before_external_changes: z.boolean().optional(),
  created_from: z.string().optional(),
  owner_set: z.boolean().optional(),
});

const CeremonyPersonalizationSeedSchema = z.object({
  text: z.string().optional(),
  source: z.string().optional(),
  createdFrom: z.string().optional(),
  ownerSet: z.boolean().optional(),
  routedTo: z.string().optional(),
  reviewable: z.boolean(),
});

export const CeremonyResponseSchema = z.object({
  journeyState: CeremonyJourneyStateSchema,
  ownerProfileSeed: CeremonyOwnerProfileSeedSchema,
  presencePreference: CeremonyPresencePreferenceSchema,
  trustBoundaryDefaults: CeremonyTrustBoundariesSchema,
  personalizationSeed: CeremonyPersonalizationSeedSchema,
  pactSummary: z.array(z.string()),
  chatId: z.string().optional(),
  redirect: z.string().optional(),
});

const InitChatResponseSchema = z.object({
  chatId: z.string().optional(),
  redirect: z.string(),
});

export type CeremonyJourneyStatus = z.infer<typeof CeremonyJourneyStatusSchema>;
export type CeremonyJourneyState = z.infer<typeof CeremonyJourneyStateSchema>;
export type CeremonyPresenceMode = z.infer<typeof CeremonyPresencePreferenceSchema>['mode'];
export type CeremonyTrustBoundaries = z.infer<typeof CeremonyTrustBoundariesSchema>;
export type CeremonyResponse = z.infer<typeof CeremonyResponseSchema>;
export type InitChatResponse = z.infer<typeof InitChatResponseSchema>;

export interface CompleteCeremonyInput {
  displayName: string;
  presenceMode: CeremonyPresenceMode;
  trustBoundaries: CeremonyTrustBoundaries;
  personalizationSeed: string;
}

export async function fetchCeremony(): Promise<CeremonyResponse> {
  return naviFetch('/api/ceremony', CeremonyResponseSchema);
}

export function useCeremony() {
  return useQuery<CeremonyResponse>({
    queryKey: CEREMONY_QUERY_KEY,
    queryFn: fetchCeremony,
    staleTime: 5_000,
  });
}

export function useStartCeremony() {
  const queryClient = useQueryClient();
  return useMutation<CeremonyResponse, Error, string>({
    mutationFn: async (currentStep) => naviFetch('/api/ceremony/start', CeremonyResponseSchema, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ current_step: currentStep }),
    }),
    onSuccess: (data) => {
      queryClient.setQueryData(CEREMONY_QUERY_KEY, data);
    },
  });
}

export function useCompleteCeremony() {
  const queryClient = useQueryClient();
  return useMutation<CeremonyResponse, Error, CompleteCeremonyInput>({
    mutationFn: async (input) => naviFetch('/api/ceremony/complete', CeremonyResponseSchema, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        display_name: input.displayName,
        presence_mode: input.presenceMode,
        trust_boundaries: input.trustBoundaries,
        personalization_seed: input.personalizationSeed,
      }),
    }),
    onSuccess: (data) => {
      queryClient.setQueryData(CEREMONY_QUERY_KEY, data);
    },
  });
}

export function useSkipCeremony() {
  const queryClient = useQueryClient();
  return useMutation<CeremonyResponse, Error, void>({
    mutationFn: async () => naviFetch('/api/ceremony/skip', CeremonyResponseSchema, {
      method: 'POST',
    }),
    onSuccess: (data) => {
      queryClient.setQueryData(CEREMONY_QUERY_KEY, data);
    },
  });
}

export function useCeremonyInitChat() {
  const queryClient = useQueryClient();
  return useMutation<InitChatResponse, Error, void>({
    mutationFn: async () => naviFetch('/api/ceremony/init-chat', InitChatResponseSchema, {
      method: 'POST',
    }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: CEREMONY_QUERY_KEY });
    },
  });
}

export interface CeremonyStepInput {
  chatId: string;
  step: string;
  value?: string | string[] | Record<string, boolean>;
  action?: string;
}

const CeremonyStepResponseSchema = z.object({
  step: z.string(),
  ok: z.boolean().optional(),
  action: z.string().optional(),
  redirect: z.string().optional(),
});

export type CeremonyStepResponse = z.infer<typeof CeremonyStepResponseSchema>;

/** Maps UI ceremony selections to the gateway POST /api/ceremony/step body. */
export function buildCeremonyStepRequestBody(input: CeremonyStepInput): Record<string, unknown> {
  const body: Record<string, unknown> = {
    chatId: input.chatId,
    step: input.step,
  };
  if (input.step === 'pact_summary') {
    const action =
      input.action ?? (typeof input.value === 'string' ? input.value : undefined);
    if (action) body.action = action;
  } else if (input.value !== undefined) {
    body.value = input.value;
  }
  return body;
}

export function useCeremonyStep() {
  const queryClient = useQueryClient();
  return useMutation<CeremonyStepResponse, Error, CeremonyStepInput>({
    mutationFn: async (input) => naviFetch('/api/ceremony/step', CeremonyStepResponseSchema, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(buildCeremonyStepRequestBody(input)),
    }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: CEREMONY_QUERY_KEY });
    },
  });
}
