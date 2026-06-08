import type { ReactNode } from 'react';
import {
  AlertTriangle,
  Activity,
  Cable,
  Cpu,
  Database,
  Gauge,
  MessageSquare,
  PlayCircle,
  Terminal,
  Wrench,
} from 'lucide-react';
import { useActivity } from '@/api/activity';
import { useLlmPreferences } from '@/api/config';
import { useRuntimeMetrics } from '@/api/debug';
import { useConnectorHealth, useSkills, useTools } from '@/api/extensions';
import { useRuns } from '@/api/runs';
import { useChats } from '@/api/chats';
import { useErrorSummary, useLlmActive, useOperatorOverview, useSystemStatus } from '@/api/status';
import { NaviApiError } from '@/api/errors';
import { EmptyState } from '@/components/ui/EmptyState';
import { JsonPanel } from '@/components/JsonPanel';
import { StatusBadge } from '@/components/ui/StatusBadge';
import { StatusCard } from '@/components/StatusCard';
import type { ErrorSummaryResponse, RunItem, RuntimeMetrics } from '@/types/api';
import styles from './Usage.module.css';

type CountRow = {
  label: string;
  count: number;
  detail?: string;
};

type UsageField = {
  path: string;
  value: number;
};

type Warning = {
  title: string;
  detail: string;
  severity: 'warning' | 'danger';
};

type ConnectorUsageItem = {
  name: string;
  state?: string;
  status?: string;
  send_count?: number;
  error_count?: number;
  consecutive_errors?: number;
};

const failedRunStatuses = new Set(['failed', 'error', 'cancelled', 'canceled', 'blocked']);
const openChatStatuses = new Set(['active', 'open', 'running', 'processing', 'idle']);
const numberFmt = new Intl.NumberFormat();

function errorMsg(err: Error | null): string | null {
  if (!err) return null;
  if (err instanceof NaviApiError) return `[${err.status}] ${err.message}`;
  return err.message;
}

export function Usage() {
  const overview = useOperatorOverview();
  const status = useSystemStatus();
  const runs = useRuns(50);
  const chats = useChats();
  const activity = useActivity(100);
  const runtimeMetrics = useRuntimeMetrics();
  const llmActive = useLlmActive();
  const llmPreferences = useLlmPreferences();
  const skills = useSkills();
  const tools = useTools();
  const connectorHealth = useConnectorHealth();
  const errors = useErrorSummary();

  const runItems = runs.data?.items ?? overview.data?.runs?.items ?? [];
  const activityItems = activity.data?.items ?? overview.data?.activity?.items ?? [];
  const chatItems = chats.data ?? [];
  const skillItems = skills.data?.items ?? [];
  const toolItems = tools.data?.items ?? [];
  const connectorItems: ConnectorUsageItem[] = (connectorHealth.data ?? status.data?.connectors.items ?? []).map((item) => ({
    name: item.name,
    state: item.state,
    status: 'status' in item ? item.status : undefined,
    send_count: item.send_count,
    error_count: item.error_count,
    consecutive_errors: 'consecutive_errors' in item ? item.consecutive_errors : undefined,
  }));
  const runtimeKeys = runtimeMetrics.data ? Object.keys(runtimeMetrics.data) : [];

  const runStatusRows = countBy(runItems, (run) => cleanStatus(run.status)).map((row) => ({
    ...row,
    detail: `${Math.round((row.count / Math.max(runItems.length, 1)) * 100)}%`,
  }));
  const chatStatusRows = countBy(chatItems, (chat) => cleanStatus(chat.status));
  const activityTypeRows = countBy(activityItems, (item) => item.type || 'unknown');
  const errorRows = normalizeErrorRows(errors.data);
  const runtimeRows = flattenMetricRows(runtimeMetrics.data).slice(0, 18);
  const usageFields = collectUsageFields({
    runtime_metrics: runtimeMetrics.data,
    runs: runItems,
    governor: status.data?.governor,
  }).slice(0, 16);

  const failedRuns = runItems.filter((run) => failedRunStatuses.has(cleanStatus(run.status))).length;
  const openChats = chatItems.filter((chat) => openChatStatuses.has(cleanStatus(chat.status))).length;
  const connectorSendTotal = sumNumeric(connectorItems, 'send_count');
  const connectorErrorTotal = sumNumeric(connectorItems, 'error_count');
  const connectorConsecutiveErrorMax = Math.max(0, ...connectorItems.map((item) => numberValue(item.consecutive_errors)));
  const enabledSkills = skillItems.filter((skill) => skill.enabled !== false).length;
  const inactiveSkills = skillItems.length - enabledSkills;
  const configuredLlm = Boolean(status.data?.llm.configured || llmActive.data?.provider || llmActive.data?.model);
  const warnings = buildWarnings({
    governorTripped: Boolean(status.data?.governor.tripped),
    connectorErrorTotal,
    connectorConsecutiveErrorMax,
    errorRows,
    failedRuns,
    totalRuns: runItems.length,
    configuredLlm,
    skillCount: skillItems.length,
    toolCount: toolItems.length,
    runtimeKnown: runtimeKeys.length > 0 && !runtimeMetrics.error,
  });

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <div>
          <h2 className={styles.heading}>Usage</h2>
          <div className={styles.subhead}>Operational visibility</div>
        </div>
        <StatusBadge
          label={warnings.length > 0 ? `${warnings.length} warnings` : 'nominal'}
          variant={warnings.some((warning) => warning.severity === 'danger') ? 'danger' : warnings.length > 0 ? 'warning' : 'success'}
          size="md"
        />
      </div>

      {warnings.length > 0 ? (
        <div className={styles.warningGrid}>
          {warnings.map((warning) => (
            <div key={warning.title} className={`${styles.warningCard} ${styles[warning.severity]}`}>
              <AlertTriangle size={16} />
              <div>
                <div className={styles.warningTitle}>{warning.title}</div>
                <div className={styles.warningDetail}>{warning.detail}</div>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className={styles.okCard}>No usage warnings from the current API snapshot.</div>
      )}

      <div className={styles.summaryGrid}>
        <MetricCard title="Recent runs" value={runItems.length} detail={`${failedRuns} failed`} />
        <MetricCard title="Recent chats" value={chatItems.length} detail={`${openChats} open`} />
        <MetricCard title="Recent activity" value={activityItems.length} detail="feed items" />
        <MetricCard title="Errors" value={errors.data?.total ?? errorRows.reduce((sum, row) => sum + row.count, 0)} detail={errors.data?.window ?? 'summary'} tone={errorRows.length > 0 ? 'danger' : 'normal'} />
        <MetricCard title="Skills" value={skillItems.length} detail={`${enabledSkills} enabled`} tone={skillItems.length === 0 ? 'warning' : 'normal'} />
        <MetricCard title="Tools" value={toolItems.length} detail="loaded" tone={toolItems.length === 0 ? 'warning' : 'normal'} />
        <MetricCard title="Connector sends" value={connectorSendTotal} detail={`${connectorErrorTotal} errors`} tone={connectorErrorTotal > 0 ? 'warning' : 'normal'} />
        <MetricCard title="Metric keys" value={runtimeKeys.length} detail="runtime snapshot" tone={runtimeKeys.length === 0 ? 'warning' : 'normal'} />
      </div>

      <div className={styles.grid}>
        <StatusCard title="Runtime Summary" loading={status.isLoading || runtimeMetrics.isLoading} error={errorMsg(status.error) ?? errorMsg(runtimeMetrics.error)} icon={<Gauge size={16} />}>
          <div className={styles.rows}>
            <Row label="Gateway" value={status.data?.gateway.version ?? status.data?.gateway.build ?? 'unknown'} />
            <Row label="Degraded">
              <StatusBadge label={status.data?.degraded ? 'degraded' : 'operational'} variant={status.data?.degraded ? 'warning' : 'success'} />
            </Row>
            <Row label="Governor">
              <StatusBadge label={status.data?.governor.tripped ? 'tripped' : 'clear'} variant={status.data?.governor.tripped ? 'danger' : 'success'} />
            </Row>
            <Row label="Budget" value={status.data?.governor ? `${status.data.governor.budget_used} / ${status.data.governor.budget_max}` : 'unknown'} />
            <Row label="Cost ledger" value={status.data?.governor ? `$${status.data.governor.cost_used.toFixed(2)} / $${status.data.governor.cost_ceiling.toFixed(2)}` : 'unknown'} />
            <Row label="Runtime keys" value={runtimeKeys.length ? numberFmt.format(runtimeKeys.length) : 'unknown'} />
          </div>
        </StatusCard>

        <StatusCard title="Runs Summary" loading={runs.isLoading} error={errorMsg(runs.error)} icon={<PlayCircle size={16} />}>
          {runItems.length > 0 ? (
            <div className={styles.stack}>
              <CompactTable rows={runStatusRows} empty="No run statuses" />
              <div className={styles.recentList}>
                {runItems.slice(0, 6).map((run) => (
                  <RunRow key={run.id ?? run.run_id ?? `${run.chat_id ?? run.runtime_session_id}-${run.created_at}`} run={run} />
                ))}
              </div>
            </div>
          ) : (
            <EmptyState icon={<PlayCircle size={20} />} message="No recent runs" />
          )}
        </StatusCard>

        <StatusCard title="Chat Summary" loading={chats.isLoading} error={errorMsg(chats.error)} icon={<MessageSquare size={16} />}>
          {chatItems.length > 0 ? (
            <div className={styles.stack}>
              <div className={styles.rows}>
                <Row label="Recent" value={numberFmt.format(chatItems.length)} />
                <Row label="Active/open" value={numberFmt.format(openChats)} />
              </div>
              <CompactTable rows={chatStatusRows} empty="No chat statuses" />
            </div>
          ) : (
            <EmptyState icon={<MessageSquare size={20} />} message="No recent chats" />
          )}
        </StatusCard>

        <StatusCard title="LLM Summary" loading={llmActive.isLoading || llmPreferences.isLoading} error={errorMsg(llmActive.error) ?? errorMsg(llmPreferences.error)} icon={<Cpu size={16} />}>
          <div className={styles.rows}>
            <Row label="Configured">
              <StatusBadge label={configuredLlm ? 'configured' : 'missing'} variant={configuredLlm ? 'success' : 'danger'} />
            </Row>
            <Row label="Provider" value={llmActive.data?.provider || 'none'} />
            <Row label="Model" value={llmActive.data?.model || 'none'} />
            <Row label="Status" value={llmActive.data?.status || status.data?.llm.status || 'unknown'} />
            <JsonPanel data={llmPreferences.data ?? {}} label="/api/llm/preferences" defaultExpanded={false} />
          </div>
        </StatusCard>

        <StatusCard title="Capability Surface" loading={skills.isLoading || tools.isLoading} error={errorMsg(skills.error) ?? errorMsg(tools.error)} icon={<Wrench size={16} />}>
          <div className={styles.rows}>
            <Row label="Skills" value={numberFmt.format(skillItems.length)} />
            <Row label="Enabled skills" value={numberFmt.format(enabledSkills)} />
            <Row label="Inactive skills" value={numberFmt.format(inactiveSkills)} />
            <Row label="Tools" value={numberFmt.format(toolItems.length)} />
            <Row label="Tool categories" value={numberFmt.format(countUnique(toolItems, (tool) => tool.category || 'uncategorized'))} />
          </div>
        </StatusCard>

        <StatusCard title="Connector Health" loading={connectorHealth.isLoading} error={errorMsg(connectorHealth.error)} icon={<Cable size={16} />}>
          {connectorItems.length > 0 ? (
            <div className={styles.stack}>
              <div className={styles.rows}>
                <Row label="Connectors" value={numberFmt.format(connectorItems.length)} />
                <Row label="Sends" value={numberFmt.format(connectorSendTotal)} />
                <Row label="Errors" value={numberFmt.format(connectorErrorTotal)} />
              </div>
              <ConnectorTable items={connectorItems} />
            </div>
          ) : (
            <EmptyState icon={<Cable size={20} />} message="No connector health data" />
          )}
        </StatusCard>

        <StatusCard title="Recent Activity Volume" loading={activity.isLoading} error={errorMsg(activity.error)} icon={<Activity size={16} />}>
          {activityItems.length > 0 ? (
            <div className={styles.stack}>
              <Row label="Recent items" value={numberFmt.format(activityItems.length)} />
              <CompactTable rows={activityTypeRows} empty="No activity types" />
            </div>
          ) : (
            <EmptyState icon={<Activity size={20} />} message="No recent activity" />
          )}
        </StatusCard>

        <StatusCard title="Error Summary" loading={errors.isLoading} error={errorMsg(errors.error)} icon={<AlertTriangle size={16} />}>
          {errorRows.length > 0 ? (
            <CompactTable rows={errorRows} empty="No errors" />
          ) : (
            <EmptyState icon={<AlertTriangle size={20} />} message="No recent errors" />
          )}
        </StatusCard>

        <StatusCard title="Runtime Metrics Snapshot" loading={runtimeMetrics.isLoading} error={errorMsg(runtimeMetrics.error)} icon={<Database size={16} />} className={styles.wide}>
          {runtimeRows.length > 0 ? (
            <MetricTable rows={runtimeRows} />
          ) : (
            <EmptyState icon={<Database size={20} />} message="Runtime metrics unavailable" />
          )}
        </StatusCard>

        <StatusCard title="Cost / Tokens" loading={runtimeMetrics.isLoading || runs.isLoading || status.isLoading} error={errorMsg(runtimeMetrics.error) ?? errorMsg(runs.error) ?? errorMsg(status.error)} icon={<Gauge size={16} />} className={styles.wide}>
          {usageFields.length > 0 ? (
            <MetricTable rows={usageFields.map((field) => ({ key: field.path, value: formatNumber(field.value) }))} />
          ) : (
            <div className={styles.unavailable}>Token/cost data unavailable from current API.</div>
          )}
        </StatusCard>
      </div>

      <section className={styles.rawSection}>
        <div className={styles.rawHeader}>
          <Terminal size={14} />
          <span>Raw Data Inspection</span>
        </div>
        <div className={styles.rawGrid}>
          <JsonPanel data={status.data ?? null} label="/api/status" />
          <JsonPanel data={runs.data ?? null} label="/api/runs" />
          <JsonPanel data={chats.data ?? null} label="/api/navi/chats" />
          <JsonPanel data={activity.data ?? null} label="/api/activity" />
          <JsonPanel data={runtimeMetrics.data ?? null} label="/api/debug/runtime-metrics" />
          <JsonPanel data={overview.data ?? null} label="/api/operator/overview" />
          <JsonPanel data={errors.data ?? null} label="/api/errors/summary" />
          <JsonPanel data={llmActive.data ?? null} label="/api/llm/active" />
          <JsonPanel data={skills.data ?? null} label="/api/skills" />
          <JsonPanel data={tools.data ?? null} label="/api/tools" />
          <JsonPanel data={connectorHealth.data ?? null} label="/api/health/connectors" />
        </div>
      </section>
    </div>
  );
}

function MetricCard({ title, value, detail, tone = 'normal' }: { title: string; value: number; detail: string; tone?: 'normal' | 'warning' | 'danger' }) {
  return (
    <div className={`${styles.metricCard} ${styles[tone]}`}>
      <div className={styles.metricLabel}>{title}</div>
      <div className={styles.metricValue}>{numberFmt.format(value)}</div>
      <div className={styles.metricDetail}>{detail}</div>
    </div>
  );
}

function Row({ label, value, children }: { label: string; value?: string; children?: ReactNode }) {
  return (
    <div className={styles.row}>
      <span className={styles.rowLabel}>{label}</span>
      <span className={styles.rowValue}>{children ?? value}</span>
    </div>
  );
}

function CompactTable({ rows, empty }: { rows: CountRow[]; empty: string }) {
  if (rows.length === 0) return <div className={styles.emptyText}>{empty}</div>;
  return (
    <table className={styles.table}>
      <tbody>
        {rows.map((row) => (
          <tr key={row.label}>
            <th>{row.label}</th>
            <td>{numberFmt.format(row.count)}</td>
            <td>{row.detail ?? ''}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function MetricTable({ rows }: { rows: Array<{ key: string; value: string }> }) {
  return (
    <table className={styles.table}>
      <tbody>
        {rows.map((row) => (
          <tr key={row.key}>
            <th>{row.key}</th>
            <td colSpan={2}>{row.value}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function ConnectorTable({ items }: { items: ConnectorUsageItem[] }) {
  return (
    <table className={styles.table}>
      <thead>
        <tr>
          <th>Name</th>
          <th>State</th>
          <th>Sends</th>
          <th>Errors</th>
        </tr>
      </thead>
      <tbody>
        {items.slice(0, 8).map((item) => (
          <tr key={item.name}>
            <th>{item.name}</th>
            <td>{item.state ?? item.status ?? 'unknown'}</td>
            <td>{numberFmt.format(numberValue(item.send_count))}</td>
            <td>{numberFmt.format(numberValue(item.error_count))}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function RunRow({ run }: { run: RunItem }) {
  const id = run.id ?? run.run_id ?? 'unknown';
  const status = cleanStatus(run.status);
  return (
    <div className={styles.runRow}>
      <span className={styles.mono}>{id}</span>
      <StatusBadge label={status} variant={statusVariant(status)} />
      <span className={styles.timeText}>{formatTimestamp(run.updated_at ?? run.created_at)}</span>
    </div>
  );
}

function buildWarnings(input: {
  governorTripped: boolean;
  connectorErrorTotal: number;
  connectorConsecutiveErrorMax: number;
  errorRows: CountRow[];
  failedRuns: number;
  totalRuns: number;
  configuredLlm: boolean;
  skillCount: number;
  toolCount: number;
  runtimeKnown: boolean;
}): Warning[] {
  const warnings: Warning[] = [];
  if (input.governorTripped) {
    warnings.push({ title: 'Governor tripped', detail: 'The hard runtime governor reports a tripped state.', severity: 'danger' });
  }
  if (input.connectorErrorTotal >= 5 || input.connectorConsecutiveErrorMax >= 3) {
    warnings.push({ title: 'High connector errors', detail: `${input.connectorErrorTotal} connector errors, max consecutive ${input.connectorConsecutiveErrorMax}.`, severity: 'warning' });
  }
  const repeatedErrors = input.errorRows.filter((row) => row.count >= 2);
  if (repeatedErrors.length > 0) {
    warnings.push({ title: 'Repeated recent errors', detail: `${repeatedErrors.length} error groups repeated in the summary window.`, severity: 'warning' });
  }
  if (input.failedRuns >= 3 || (input.totalRuns >= 5 && input.failedRuns / input.totalRuns >= 0.4)) {
    warnings.push({ title: 'Many failed runs', detail: `${input.failedRuns} of ${input.totalRuns} recent runs are failed-like states.`, severity: 'danger' });
  }
  if (!input.configuredLlm) {
    warnings.push({ title: 'No LLM configured', detail: 'Active provider/model state is missing from the current API snapshot.', severity: 'danger' });
  }
  if (input.skillCount + input.toolCount === 0) {
    warnings.push({ title: 'No skills/tools loaded', detail: 'The capability surface is empty.', severity: 'warning' });
  }
  if (!input.runtimeKnown) {
    warnings.push({ title: 'Runtime metrics unknown', detail: 'Runtime metrics are missing, empty, or unavailable.', severity: 'warning' });
  }
  return warnings;
}

function countBy<T>(items: T[], getLabel: (item: T) => string): CountRow[] {
  const counts = new Map<string, number>();
  for (const item of items) {
    const label = getLabel(item) || 'unknown';
    counts.set(label, (counts.get(label) ?? 0) + 1);
  }
  return [...counts.entries()]
    .map(([label, count]) => ({ label, count }))
    .sort((a, b) => b.count - a.count || a.label.localeCompare(b.label));
}

function normalizeErrorRows(summary?: ErrorSummaryResponse): CountRow[] {
  return (summary?.items ?? [])
    .map((item) => ({
      label: [item.type || 'unknown_type', item.component || 'unknown_component'].join(' / '),
      count: item.count,
      detail: item.last_seen ? formatTimestamp(item.last_seen) : undefined,
    }))
    .sort((a, b) => b.count - a.count || a.label.localeCompare(b.label));
}

function cleanStatus(status: unknown): string {
  return typeof status === 'string' && status.trim() ? status.trim().toLowerCase() : 'unknown';
}

function statusVariant(status: string): 'success' | 'warning' | 'danger' | 'accent' | 'muted' {
  if (failedRunStatuses.has(status)) return 'danger';
  if (status === 'completed' || status === 'complete' || status === 'succeeded' || status === 'success') return 'success';
  if (status === 'running' || status === 'active') return 'accent';
  if (status === 'pending' || status === 'queued') return 'warning';
  return 'muted';
}

function countUnique<T>(items: T[], getValue: (item: T) => string): number {
  return new Set(items.map(getValue)).size;
}

function sumNumeric<T>(items: T[], key: string): number {
  return items.reduce((sum, item) => sum + numberValue((item as Record<string, unknown>)[key]), 0);
}

function numberValue(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
}

function flattenMetricRows(metrics?: RuntimeMetrics): Array<{ key: string; value: string }> {
  if (!metrics) return [];
  return Object.entries(metrics)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([key, value]) => ({ key, value: formatMetricValue(value) }));
}

function formatMetricValue(value: unknown): string {
  if (typeof value === 'number') return formatNumber(value);
  if (typeof value === 'string') return value;
  if (typeof value === 'boolean') return value ? 'true' : 'false';
  if (value === null || value === undefined) return 'null';
  try {
    return JSON.stringify(value);
  } catch {
    return '[unserializable]';
  }
}

function collectUsageFields(value: unknown, prefix = ''): UsageField[] {
  const fields: UsageField[] = [];
  visitUsage(value, prefix, fields, 0);
  return fields.sort((a, b) => a.path.localeCompare(b.path));
}

function visitUsage(value: unknown, prefix: string, fields: UsageField[], depth: number) {
  if (depth > 5 || value === null || value === undefined) return;
  if (Array.isArray(value)) {
    value.slice(0, 25).forEach((item, index) => visitUsage(item, `${prefix}[${index}]`, fields, depth + 1));
    return;
  }
  if (typeof value !== 'object') return;

  for (const [key, next] of Object.entries(value as Record<string, unknown>)) {
    const path = prefix ? `${prefix}.${key}` : key;
    if (typeof next === 'number' && Number.isFinite(next) && /(token|cost|usd)/i.test(key)) {
      fields.push({ path, value: next });
      continue;
    }
    visitUsage(next, path, fields, depth + 1);
  }
}

function formatNumber(value: number): string {
  return Number.isInteger(value) ? numberFmt.format(value) : numberFmt.format(Number(value.toFixed(4)));
}

function formatTimestamp(value?: string): string {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}
