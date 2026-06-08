import { useState } from 'react';
import { useProposals, useResolveProposal } from '@/api/proposals';
import { EmptyState } from '@/components/ui/EmptyState';
import { StatusBadge } from '@/components/ui/StatusBadge';
import { JsonPanel } from '@/components/JsonPanel';
import { GitPullRequest, CheckCircle, XCircle, Layers } from 'lucide-react';
import styles from './Proposals.module.css';
import { type Proposal } from '@/types/api';
import { proposalSource, isGroupedBackfill, groupedCount, sourceTagVariant } from '@/lib/proposalSource';

function renderJsonOrString(data: any) {
  if (!data) return '-';
  if (typeof data === 'string') {
    try {
      const parsed = JSON.parse(data);
      return <JsonPanel data={parsed} defaultExpanded />;
    } catch {
      return <div className={styles.textContent}>{data}</div>;
    }
  }
  return <JsonPanel data={data} defaultExpanded />;
}

export function Proposals() {
  const { data: proposals, error: proposalsError, isLoading: proposalsLoading } = useProposals();
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const proposalList = proposals ?? [];

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h2 className={styles.heading}>Proposals</h2>
      </div>

      <div className={styles.layout}>
        {/* Left Pane: Proposal List */}
        <div className={styles.listPane}>
          {proposalsLoading && <div className={styles.loadingRow}>Loading proposals...</div>}
          {proposalsError && (
            <div className={styles.errorRow}>
              Failed to load: {proposalsError instanceof Error ? proposalsError.message : 'Unknown error'}
            </div>
          )}
          {!proposalsLoading && !proposalsError && proposalList.length === 0 && (
            <div className={styles.emptyStateWrap}>
              <EmptyState icon={<GitPullRequest size={24} />} message="No pending proposals" />
            </div>
          )}
          {proposalList.map((p) => {
            const id = p.proposal_id ?? p.id ?? 'unknown';
            const isActive = id === selectedId;
            const source = proposalSource(p);
            const grouped = isGroupedBackfill(p);
            const count = grouped ? groupedCount(p) : 0;
            return (
              <div
                key={id}
                className={`${styles.proposalItem} ${isActive ? styles.active : ''}`}
                onClick={() => setSelectedId(id)}
              >
                <div className={styles.proposalTitle}>{id}</div>
                <div className={styles.proposalSummary}>
                  {grouped && count > 0 ? (
                    <span><Layers size={12} /> {count} item group · {p.proposed_action ?? 'group'}</span>
                  ) : (
                    p.proposed_action ?? 'Unknown Action'
                  )}
                </div>
                <div className={styles.proposalMeta}>
                  <StatusBadge
                    label={p.status ?? 'unknown'}
                    variant={p.status === 'pending' ? 'accent' : p.status === 'approved' ? 'success' : 'muted'}
                  />
                  {source && <StatusBadge label={source} variant={sourceTagVariant(source)} />}
                  {grouped && <StatusBadge label="grouped" variant="info" />}
                  <span>
                    {p.created_at
                      ? new Date(p.created_at).toLocaleDateString(undefined, {
                          month: 'short',
                          day: 'numeric',
                          hour: '2-digit',
                          minute: '2-digit',
                        })
                      : 'Unknown date'}
                  </span>
                </div>
              </div>
            );
          })}
        </div>

        {/* Right Pane: Selected Proposal Details */}
        <div className={styles.detailPane}>
          {selectedId ? (
            <ProposalDetail id={selectedId} proposal={proposalList.find((p) => (p.proposal_id ?? p.id) === selectedId)} />
          ) : (
            <div className={styles.emptyStateWrap}>
              <EmptyState icon={<GitPullRequest size={32} />} message="Select a proposal" />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function ProposalDetail({ id, proposal }: { id: string; proposal?: Proposal }) {
  const resolveMutation = useResolveProposal();
  const [note, setNote] = useState('');
  const [resolveError, setResolveError] = useState<string | null>(null);

  if (!proposal) {
    return (
      <div className={styles.emptyStateWrap}>
        <EmptyState icon={<GitPullRequest size={32} />} message="Proposal not found or no longer pending." />
      </div>
    );
  }

  const sId = proposal.proposal_id ?? proposal.id ?? id;
  const status = proposal.status ?? 'unknown';
  const isPending = status === 'pending';
  const detailSource = proposalSource(proposal);
  const detailGrouped = isGroupedBackfill(proposal);
  const detailCount = detailGrouped ? groupedCount(proposal) : 0;

  const handleResolve = async (action: 'approve' | 'decline') => {
    setResolveError(null);
    try {
      await resolveMutation.mutateAsync({
        id: sId,
        action,
        resolution_type: action === 'approve' ? 'approved_once' : 'denied',
        resolution_note: note,
      });
      setNote('');
    } catch (err) {
      setResolveError(err instanceof Error ? err.message : String(err));
    }
  };

  return (
    <>
      <div className={styles.detailHeader}>
        <div>
          <h3 className={styles.detailTitle}>{sId}</h3>
          <div className={styles.badgeList}>
            <StatusBadge label={status} variant={status === 'pending' ? 'accent' : status === 'approved' ? 'success' : 'muted'} />
            {proposal.priority && <StatusBadge label={`pri: ${proposal.priority}`} variant="muted" />}
            {detailSource && <StatusBadge label={`source: ${detailSource}`} variant={sourceTagVariant(detailSource)} />}
            {!detailSource && proposal.source_process && <StatusBadge label={`src: ${proposal.source_process}`} variant="muted" />}
            {detailGrouped && <StatusBadge label="grouped backfill" variant="info" />}
          </div>
        </div>
      </div>

      {detailGrouped && (
        <div className={styles.section}>
          <div className={styles.sectionLabel}>Grouped backfill</div>
          <div className={styles.textContent}>
            {detailCount > 0 ? `${detailCount} ` : ''}items batched from a backfill — approve or decline the
            whole set in one action (synthesis seam §12). Resolving this Proposal resolves every member.
          </div>
        </div>
      )}

      {resolveError && (
        <div className={styles.errorRow}>
          Resolution failed: {resolveError}
        </div>
      )}

      {isPending && (
        <div className={styles.resolutionArea}>
          <h4>Resolve Proposal</h4>
          <textarea
            className={styles.textarea}
            placeholder="Optional resolution note/reason..."
            value={note}
            onChange={(e) => setNote(e.target.value)}
            disabled={resolveMutation.isPending}
          />
          <div className={styles.actions}>
            <button
              className={styles.declineBtn}
              onClick={() => handleResolve('decline')}
              disabled={resolveMutation.isPending}
            >
              <XCircle size={14} /> Decline
            </button>
            <button
              className={styles.approveBtn}
              onClick={() => handleResolve('approve')}
              disabled={resolveMutation.isPending}
            >
              <CheckCircle size={14} /> Approve
            </button>
          </div>
        </div>
      )}

      <div className={styles.section}>
        <div className={styles.sectionLabel}>Metadata</div>
        <div className={styles.infoGrid}>
          <div className={styles.infoRow}>
            <span className={styles.infoLabel}>Source Trigger</span>
            <span className={styles.infoValue}>{proposal.source_trigger || '-'}</span>
          </div>
          <div className={styles.infoRow}>
            <span className={styles.infoLabel}>Boundary Key</span>
            <span className={styles.infoValue}>{proposal.boundary_key || '-'}</span>
          </div>
          <div className={styles.infoRow}>
            <span className={styles.infoLabel}>Created At</span>
            <span className={styles.infoValue}>
              {proposal.created_at ? new Date(proposal.created_at).toLocaleString() : '-'}
            </span>
          </div>
          <div className={styles.infoRow}>
            <span className={styles.infoLabel}>Expires At</span>
            <span className={styles.infoValue}>
              {proposal.expires_at ? new Date(proposal.expires_at).toLocaleString() : '-'}
            </span>
          </div>
        </div>
      </div>

      {(proposal.resolved_at || proposal.resolved_by || proposal.resolution_type || proposal.resolution_note) && (
        <div className={styles.section}>
          <div className={styles.sectionLabel}>Resolution State</div>
          <div className={styles.infoGrid}>
            {proposal.resolved_at && (
              <div className={styles.infoRow}>
                <span className={styles.infoLabel}>Resolved At</span>
                <span className={styles.infoValue}>{new Date(proposal.resolved_at).toLocaleString()}</span>
              </div>
            )}
            {proposal.resolved_by && (
              <div className={styles.infoRow}>
                <span className={styles.infoLabel}>Resolved By</span>
                <span className={styles.infoValue}>{proposal.resolved_by}</span>
              </div>
            )}
            {proposal.resolution_type && (
              <div className={styles.infoRow}>
                <span className={styles.infoLabel}>Type</span>
                <span className={styles.infoValue}>{proposal.resolution_type}</span>
              </div>
            )}
          </div>
          {proposal.resolution_note && (
            <div className={styles.textContent}>{proposal.resolution_note}</div>
          )}
        </div>
      )}

      {proposal.rationale && (
        <div className={styles.section}>
          <div className={styles.sectionLabel}>Rationale</div>
          <div className={styles.textContent}>{proposal.rationale}</div>
        </div>
      )}

      {proposal.proposed_action && (
        <div className={styles.section}>
          <div className={styles.sectionLabel}>Proposed Action</div>
          {renderJsonOrString(proposal.proposed_action)}
        </div>
      )}

      {proposal.affected_entities && proposal.affected_entities.length > 0 && (
        <div className={styles.section}>
          <div className={styles.sectionLabel}>Affected Entities</div>
          {renderJsonOrString(proposal.affected_entities)}
        </div>
      )}

      <div className={styles.section}>
        <div className={styles.sectionLabel}>Raw Proposal</div>
        <JsonPanel data={proposal} label="Payload" />
      </div>
    </>
  );
}
