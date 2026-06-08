import { type Proposal } from '@/types/api';

// CIP P5 — derive the Console `source:` tag for a Proposal from its
// source_process. All intake/Vault Proposals flow through the ONE existing
// Proposal queue, distinguished by this tag (Console V2 addendum; no second
// queue).
export type ProposalSource = 'intake-consent' | 'intake-synthesis' | 'vault' | null;

export function proposalSource(p: Proposal): ProposalSource {
  switch (p.source_process) {
    case 'intake_consent':
      return 'intake-consent';
    case 'intake_synthesis':
      return 'intake-synthesis';
    case 'vault':
      return 'vault';
    default:
      return null;
  }
}

// isGroupedBackfill reports whether a Proposal is a backfill-batched group
// ("N contact merges from … backfill — review as a set"), per synthesis seam
// §12. Such Proposals get a group affordance and approve/reject as a set.
export function isGroupedBackfill(p: Proposal): boolean {
  const action = p.proposed_action ?? '';
  const trigger = p.source_trigger ?? '';
  return action.startsWith('merge_contacts_group:') || trigger.endsWith(':backfill');
}

// groupedCount extracts the member count of a grouped Proposal from its
// affected_entities, falling back to a leading integer in the rationale.
export function groupedCount(p: Proposal): number {
  if (Array.isArray(p.affected_entities) && p.affected_entities.length > 0) {
    return p.affected_entities.length;
  }
  const m = /^(\d+)\s/.exec(p.rationale ?? '');
  return m ? parseInt(m[1], 10) : 0;
}

export function sourceTagVariant(source: ProposalSource): 'accent' | 'success' | 'muted' {
  switch (source) {
    case 'intake-consent':
      return 'accent';
    case 'intake-synthesis':
      return 'success';
    case 'vault':
      return 'muted';
    default:
      return 'muted';
  }
}
