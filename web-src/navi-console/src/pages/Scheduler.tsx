import { usePlugins, useSkills } from '@/api/extensions';
import { useActivity } from '@/api/activity';
import { useRuns } from '@/api/runs';
import { useDebugEvents, useRuntimeMetrics } from '@/api/debug';
import { useCapabilitiesGraph } from '@/api/capabilities';
import { StatusCard } from '@/components/StatusCard';
import { StatusBadge } from '@/components/ui/StatusBadge';
import { JsonPanel } from '@/components/JsonPanel';
import { EmptyState } from '@/components/ui/EmptyState';
import { useState } from 'react';
import { 
  Activity, 
  Bug, 
  PlayCircle, 
  BarChart3, 
  Terminal,
  ShieldAlert,
  Zap,
  Puzzle,
  Wrench,
  ChevronDown,
  ChevronRight
} from 'lucide-react';
import styles from './Scheduler.module.css';

const SCHEDULER_TERMS = ['scheduler', 'schedule', 'cron', 'timer', 'job', 'trigger'];

function matchesScheduler(text: string): boolean {
  const lower = text.toLowerCase();
  return SCHEDULER_TERMS.some(term => lower.includes(term));
}

function errorMsg(err: Error | null): string | null {
  return err ? err.message : null;
}

function metadataEnabled(value: { enabled?: boolean; active?: boolean } | undefined): boolean {
  if (!value) return false;
  return value.enabled !== false && value.active !== false;
}

export function Scheduler() {
  const plugins = usePlugins();
  const skills = useSkills();
  const activity = useActivity();
  const runs = useRuns();
  const debugEvents = useDebugEvents();
  const runtimeMetrics = useRuntimeMetrics();
  const graph = useCapabilitiesGraph();

  const [showGapDetails, setShowGapDetails] = useState(false);
  const graphUnavailable = !graph.isLoading && (graph.isError || graph.data == null);
  const graphUnavailableReason = graph.error instanceof Error ? graph.error.message : 'capability graph returned no data';

  // Readiness signals (Manifest based)
  const schedulerPluginManifest = plugins.data?.plugins?.find(p => matchesScheduler(p.id || '') || matchesScheduler(p.name || ''));
  const schedulerSkillMetadata = skills.data?.items?.find(s => matchesScheduler(s.id || '') || matchesScheduler(s.name || ''));
  
  // Capability Graph based signals
  const schedulerPluginNode = graphUnavailable ? undefined : graph.data?.plugins?.find(p => matchesScheduler(p.id || '') || matchesScheduler(p.displayName || ''));
  const schedulerSkillNode = graphUnavailable ? undefined : graph.data?.skills?.find(s => matchesScheduler(s.id || '') || matchesScheduler(s.displayName || ''));
  const schedulerToolNodes = graphUnavailable ? undefined : graph.data?.toolInterfaces?.filter(t =>
    matchesScheduler(t.id || '') || 
    matchesScheduler(t.displayName || '') ||
    matchesScheduler(t.skillId || '') ||
    matchesScheduler(t.registeredToolName || '')
  );

  // Fallback readiness logic
  const pluginDetected = !!(schedulerPluginNode || schedulerPluginManifest);
  const skillDetected = !!(schedulerSkillNode || schedulerSkillMetadata);

  const pluginEnabled = schedulerPluginNode 
    ? schedulerPluginNode.status.availability === 'available'
    : metadataEnabled(schedulerPluginManifest);

  const skillAvailable = schedulerSkillNode
    ? schedulerSkillNode.status.availability === 'available'
    : metadataEnabled(schedulerSkillMetadata);
  
  let toolsInvokable: 'Yes' | 'No' | 'Unknown' = 'No';
  if (graphUnavailable) {
    toolsInvokable = 'Unknown';
  } else if (pluginEnabled && skillAvailable && (schedulerToolNodes?.length ?? 0) > 0) {
    toolsInvokable = 'Yes';
  }

  // Filtered data
  const filteredActivity = activity.data?.items?.filter(a => 
    matchesScheduler(a.summary) || matchesScheduler(a.type)
  );

  const filteredDebugEvents = debugEvents.data?.items?.filter(e => 
    matchesScheduler(e.type || '') || 
    (e.payload && matchesScheduler(JSON.stringify(e.payload)))
  );

  const filteredRuns = Array.isArray(runs.data?.items) ? (runs.data?.items as any[]).filter((r: any) => 
    matchesScheduler(r.source || '') || 
    matchesScheduler(r.trigger_type || '') ||
    matchesScheduler(r.title || '')
  ) : [];

    const schedulerMetrics = runtimeMetrics.data ? Object.entries(runtimeMetrics.data).filter(([key]) => 
    matchesScheduler(key)
  ) : [];

  return (
    <div className={styles.page}>
      <h2 className={styles.heading}>Scheduler</h2>

      <div className={styles.grid}>
        {/* Graph Availability Warning */}
        {graphUnavailable && (
          <div className={styles.wide}>
            <div className={styles.warningBanner}>
              <Zap size={20} className={styles.warningIcon} />
              <div className={styles.warningText}>
                <strong>Capability Graph Unavailable</strong>
                <span>
                  The advanced capability graph is currently unavailable. Scheduler is operating in degraded mode using local plugin/skill manifests.
                  Tool-level invokability cannot be verified.
                </span>
                <span className={styles.mono}>{graphUnavailableReason}</span>
              </div>
            </div>
          </div>
        )}

        {/* Management Status - ALWAYS UNAVAILABLE */}
        <div className={styles.wide}>
          <div className={styles.managementPanel}>
            <div className={styles.managementHeader}>
              <ShieldAlert size={24} />
              <span className={styles.managementTitle}>Management Unavailable</span>
            </div>
            <p className={styles.managementDescription}>
              The Scheduler CRUD API is currently not available in the backend gateway. 
              Direct management of scheduled tasks (create, edit, delete, enable/disable) requires additional backend route support.
            </p>
            
            <button 
              className={styles.gapToggle} 
              onClick={() => setShowGapDetails(!showGapDetails)}
            >
              {showGapDetails ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
              <span>Backend Gap Details</span>
            </button>

            {showGapDetails && (
              <div className={styles.gapContent}>
                <div className={styles.rows}>
                  <span className={styles.rowLabel}>Missing Implementation Checklist:</span>
                  <ul className={styles.gapList}>
                    <li>GET /api/scheduler/tasks - List scheduled jobs</li>
                    <li>POST /api/scheduler/tasks - Register new job</li>
                    <li>PATCH /api/scheduler/tasks/&#123;id&#125; - Update job/state</li>
                    <li>DELETE /api/scheduler/tasks/&#123;id&#125; - Remove job</li>
                  </ul>
                </div>
              </div>
            )}
          </div>
        </div>

        {/* Scheduler Readiness */}
        <StatusCard title="Scheduler Readiness" icon={<Zap size={16} />} loading={plugins.isLoading || skills.isLoading || graph.isLoading} error={errorMsg(plugins.error) ?? errorMsg(skills.error)}>
          <div className={styles.rows}>
            <Row label="Management API">
              <StatusBadge label="Unavailable" variant="danger" size="sm" />
            </Row>
            <Row label="Plugin Detected">
              {pluginDetected ? (
                <StatusBadge label="Detected" variant="success" size="sm" />
              ) : (
                <StatusBadge label="Not Found" variant="muted" size="sm" />
              )}
            </Row>
            <Row label="Plugin Enabled">
              <StatusBadge 
                label={pluginEnabled ? 'Enabled' : 'Disabled'} 
                variant={pluginEnabled ? 'success' : 'warning'} 
                size="sm" 
              />
            </Row>
            <Row label="Skill Detected">
              {skillDetected ? (
                <StatusBadge label="Detected" variant="success" size="sm" />
              ) : (
                <StatusBadge label="Not Found" variant="muted" size="sm" />
              )}
            </Row>
            <Row label="Skill Available">
              <StatusBadge
                label={skillAvailable ? 'Available' : 'Unavailable'}
                variant={skillAvailable ? 'success' : 'muted'}
                size="sm"
              />
            </Row>
            <Row label="Tools Invokable">
              {toolsInvokable === 'Yes' && <StatusBadge label="Yes" variant="accent" size="sm" />}
              {toolsInvokable === 'No' && <StatusBadge label="No" variant="danger" size="sm" />}
              {toolsInvokable === 'Unknown' && <StatusBadge label="Unavailable" variant="muted" size="sm" />}
            </Row>
          </div>
        </StatusCard>

        {/* Plugin/Skill Details */}
        <StatusCard title="Capability Signals" icon={<Puzzle size={16} />} loading={plugins.isLoading || skills.isLoading || graph.isLoading} error={errorMsg(plugins.error) ?? errorMsg(skills.error)}>
          <div className={styles.rows}>
            {schedulerPluginNode && <Row label="Plugin ID" value={schedulerPluginNode.id} />}
            {!schedulerPluginNode && schedulerPluginManifest && <Row label="Plugin Manifest" value={schedulerPluginManifest.id} />}
            {schedulerSkillNode && <Row label="Skill ID" value={schedulerSkillNode.id} />}
            {!schedulerSkillNode && schedulerSkillMetadata && <Row label="Skill Metadata" value={schedulerSkillMetadata.id} />}
            {schedulerSkillNode && (
              <Row label="Status">
                <StatusBadge
                  label={schedulerSkillNode.status.availability}
                  variant={skillAvailable ? 'success' : 'warning'}
                  size="sm"
                />
              </Row>
            )}
            {!schedulerPluginNode && !schedulerSkillNode && !schedulerPluginManifest && !schedulerSkillMetadata && (
              <EmptyState icon={<Puzzle size={20} />} message="No scheduler capabilities detected" />
            )}
          </div>
        </StatusCard>

        {/* Tool Signals */}
        <StatusCard title="Discovered Tools" icon={<Wrench size={16} />} loading={graph.isLoading}>
          {graphUnavailable ? (
            <EmptyState icon={<Wrench size={20} />} message="Tool invokability unavailable while capability graph is degraded" />
          ) : schedulerToolNodes && schedulerToolNodes.length > 0 ? (
            <div className={styles.toolList}>
              {schedulerToolNodes.map(t => {
                const canonicalId = t.skillId && t.interfaceName ? `${t.skillId}.${t.interfaceName}` : t.canonicalInterfaceId;
                const isBlocked = !pluginEnabled || !skillAvailable || t.status.availability !== 'available';
                
                return (
                  <div key={t.id} className={styles.toolItem}>
                    <div className={styles.toolHeader}>
                      <span className={styles.toolCanonical}>{canonicalId}</span>
                      <StatusBadge 
                        label={isBlocked ? (pluginEnabled ? 'Blocked' : 'Plugin Disabled') : 'Ready'} 
                        variant={isBlocked ? 'warning' : 'success'} 
                        size="sm" 
                      />
                    </div>
                    {t.registeredToolName && t.registeredToolName !== t.interfaceName && (
                      <div className={styles.toolAlias}>
                        <span>Alias:</span>
                        <span className={styles.mono}>{t.registeredToolName}</span>
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          ) : (
            <EmptyState icon={<Wrench size={20} />} message="No scheduler tools found" />
          )}
        </StatusCard>

        {/* Recent Activity */}
        <StatusCard title="Recent Activity" icon={<Activity size={16} />} loading={activity.isLoading} error={errorMsg(activity.error)} className={styles.wide}>
          {filteredActivity && filteredActivity.length > 0 ? (
            <div className={styles.activityFeed}>
              {filteredActivity.slice(0, 5).map((a, i) => (
                <div key={i} className={styles.activityRow}>
                  <span className={styles.activityTime}>{new Date(a.at).toLocaleTimeString()}</span>
                  <span className={styles.activityType}>{a.type}</span>
                  <span className={styles.activitySummary}>{a.summary}</span>
                </div>
              ))}
            </div>
          ) : (
            <EmptyState icon={<Activity size={20} />} message="No scheduler activity found" />
          )}
        </StatusCard>

        {/* Debug Events */}
        <StatusCard title="Debug Events" icon={<Bug size={16} />} loading={debugEvents.isLoading} error={errorMsg(debugEvents.error)}>
          {filteredDebugEvents && filteredDebugEvents.length > 0 ? (
            <div className={styles.rows}>
              <Row label="Events Found" value={String(filteredDebugEvents.length)} />
              <JsonPanel data={filteredDebugEvents.slice(0, 3)} label="Latest Events" defaultExpanded={false} />
            </div>
          ) : (
            <EmptyState icon={<Bug size={20} />} message="No scheduler debug events" />
          )}
        </StatusCard>

        {/* Triggered Runs */}
        <StatusCard title="Triggered Runs" icon={<PlayCircle size={16} />} loading={runs.isLoading} error={errorMsg(runs.error)}>
          {filteredRuns && filteredRuns.length > 0 ? (
            <div className={styles.rows}>
              <Row label="Runs Found" value={String(filteredRuns.length)} />
              <JsonPanel data={filteredRuns.slice(0, 3)} label="Latest Runs" defaultExpanded={false} />
            </div>
          ) : (
            <EmptyState icon={<PlayCircle size={20} />} message="No scheduler-triggered runs" />
          )}
        </StatusCard>

        {/* Runtime Metrics */}
        <StatusCard title="Runtime Metrics" icon={<BarChart3 size={16} />} loading={runtimeMetrics.isLoading} error={errorMsg(runtimeMetrics.error)}>
          {schedulerMetrics.length > 0 ? (
            <div className={styles.metricsList}>
              {schedulerMetrics.map(([key, val]) => (
                <div key={key} className={styles.metricRow}>
                  <span className={styles.metricKey}>{key}</span>
                  <span className={styles.metricValue}>{String(val)}</span>
                </div>
              ))}
            </div>
          ) : (
            <EmptyState icon={<BarChart3 size={20} />} message="No scheduler runtime metrics exposed" />
          )}
        </StatusCard>
      </div>

      {/* Raw Inspection Section */}
      <div className={styles.inspection}>
        <div className={styles.inspectionHeader}>
          <Terminal size={14} />
          <span>Raw Inspection</span>
        </div>
        <div className={styles.inspectionGrid}>
          <JsonPanel data={schedulerPluginManifest} label="Plugin Manifest" />
          <JsonPanel data={schedulerPluginNode} label="Plugin Node (Graph)" />
          <JsonPanel data={schedulerSkillMetadata} label="Skill Metadata" />
          <JsonPanel data={schedulerSkillNode} label="Skill Node (Graph)" />
          <JsonPanel data={schedulerToolNodes} label="Discovered Tools" />
          <JsonPanel data={filteredActivity} label="Filtered Activity" />
          <JsonPanel data={filteredDebugEvents} label="Filtered Events" />
          <JsonPanel data={filteredRuns} label="Filtered Runs" />
          <JsonPanel data={runtimeMetrics.data} label="All Metrics" />
        </div>
      </div>
      
      <p className={styles.footerNote}>
        Scheduler Console V1 is inspection/readiness-only. Scheduled-task CRUD requires future backend gateway routes.
      </p>
    </div>
  );
}

function Row({ label, value, children }: { label: string; value?: string; children?: React.ReactNode }) {
  return (
    <div className={styles.row}>
      <span className={styles.rowLabel}>{label}</span>
      <span className={styles.rowValue}>{children ?? value}</span>
    </div>
  );
}
