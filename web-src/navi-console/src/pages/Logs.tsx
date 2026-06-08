import { useState, useMemo } from 'react';
import { ChevronDown, ChevronRight } from 'lucide-react';

import { useLiveEventsContext } from '@/hooks/LiveEventsContext';
import { useActivity, useErrors } from '@/api/activity';
import { useDebugEvents, useRuntimeMetrics } from '@/api/debug';
import { useErrorSummary } from '@/api/status';


import { FilterBar, type FilterTab } from '@/components/FilterBar';
import { EventFeed } from '@/components/EventFeed';
import { JsonPanel } from '@/components/JsonPanel';
import { StatusBadge } from '@/components/ui/StatusBadge';

import {
  activityToTimelineItem,
  debugEventToTimelineItem,
  errorToTimelineItem,
  liveEventToTimelineItem,
  mergeTimeline,
  type TimelineItem,
} from '@/types/timeline';

import styles from './Logs.module.css';

type FilterKey = 'all' | 'events' | 'activity' | 'errors';

export function Logs() {
  const live = useLiveEventsContext();
  const activity = useActivity();
  const debugEvents = useDebugEvents();
  const errors = useErrors();
  const errorSummary = useErrorSummary();
  const metrics = useRuntimeMetrics();

  const [activeTab, setActiveTab] = useState<FilterKey>('all');
  const [search, setSearch] = useState('');
  const [metricsOpen, setMetricsOpen] = useState(false);
  const [summaryOpen, setSummaryOpen] = useState(false);

  // Map each source into timeline items.
  const liveItems = useMemo<TimelineItem[]>(
    () => live.events.map(liveEventToTimelineItem),
    [live.events],
  );

  const activityItems = useMemo<TimelineItem[]>(
    () => (activity.data?.items ?? []).map(activityToTimelineItem),
    [activity.data],
  );

  const debugItems = useMemo<TimelineItem[]>(
    () => (debugEvents.data?.items ?? []).map(debugEventToTimelineItem),
    [debugEvents.data],
  );

  const errorItems = useMemo<TimelineItem[]>(
    () => (errors.data?.items ?? []).map(errorToTimelineItem),
    [errors.data],
  );

  // Merge and filter.
  const allItems = useMemo(
    () => mergeTimeline(liveItems, activityItems, debugItems, errorItems),
    [liveItems, activityItems, debugItems, errorItems],
  );

  const filteredItems = useMemo(() => {
    let items = allItems;

    // Tab filter.
    switch (activeTab) {
      case 'events':
        items = items.filter((i) => i.source === 'live' || i.source === 'debug');
        break;
      case 'activity':
        items = items.filter((i) => i.source === 'activity');
        break;
      case 'errors':
        items = items.filter((i) => i.source === 'error');
        break;
    }

    // Text search filter.
    if (search.trim()) {
      const q = search.toLowerCase();
      items = items.filter(
        (i) =>
          i.type.toLowerCase().includes(q) ||
          i.summary.toLowerCase().includes(q) ||
          i.source.toLowerCase().includes(q),
      );
    }

    return items;
  }, [allItems, activeTab, search]);

  // Counts for tabs.
  const tabs: FilterTab[] = [
    { key: 'all', label: 'All', count: allItems.length },
    { key: 'events', label: 'Events', count: allItems.filter((i) => i.source === 'live' || i.source === 'debug').length },
    { key: 'activity', label: 'Activity', count: activityItems.length },
    { key: 'errors', label: 'Errors', count: errorItems.length },
  ];

  const summaryItems = errorSummary.data?.items;
  const hasMetrics = metrics.data && Object.keys(metrics.data).length > 0;

  return (
    <div className={styles.page}>
      {/* Header */}
        <div className={styles.titleInfo}>
          <h1 className={styles.title}>Event Logs</h1>
        </div>

      {/* Collapsible panels: metrics + error summary */}
      <div className={styles.panels}>
        {/* Runtime Metrics */}
        <div>
          <button
            className={styles.panelToggle}
            onClick={() => setMetricsOpen(!metricsOpen)}
            aria-expanded={metricsOpen}
          >
            {metricsOpen ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
            Runtime Metrics
            {metrics.isLoading && <StatusBadge label="loading" variant="muted" />}
            {metrics.error && <StatusBadge label="error" variant="danger" />}
          </button>
          {metricsOpen && (
            <div className={styles.panelBody}>
              {hasMetrics ? (
                <JsonPanel data={metrics.data} label="Metrics snapshot" defaultExpanded />
              ) : metrics.isLoading ? (
                <div className={styles.loadingRow}>Loading metrics…</div>
              ) : metrics.error ? (
                <div className={styles.errorRow}>
                  Failed to load: {metrics.error instanceof Error ? metrics.error.message : 'Unknown error'}
                </div>
              ) : (
                <div className={styles.summaryEmpty}>No metrics available</div>
              )}
            </div>
          )}
        </div>

        {/* Error Summary */}
        <div>
          <button
            className={styles.panelToggle}
            onClick={() => setSummaryOpen(!summaryOpen)}
            aria-expanded={summaryOpen}
          >
            {summaryOpen ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
            Error Summary
            {errorSummary.data?.window && (
              <StatusBadge label={errorSummary.data.window} variant="muted" />
            )}
            {errorSummary.isLoading && <StatusBadge label="loading" variant="muted" />}
          </button>
          {summaryOpen && (
            <div className={styles.panelBody}>
              {summaryItems && summaryItems.length > 0 ? (
                <div className={styles.summaryGrid}>
                  {summaryItems.map((item, i) => (
                    <div key={i} className={styles.summaryItem}>
                      <span className={styles.summaryType}>
                        {item.type ?? item.component ?? 'unknown'}
                      </span>
                      <span className={styles.summaryCount}>{item.count}</span>
                    </div>
                  ))}
                </div>
              ) : errorSummary.isLoading ? (
                <div className={styles.loadingRow}>Loading summary…</div>
              ) : (
                <div className={styles.summaryEmpty}>No errors in window</div>
              )}
            </div>
          )}
        </div>
      </div>

      {/* Filter bar */}
      <FilterBar
        tabs={tabs}
        activeTab={activeTab}
        onTabChange={(key) => setActiveTab(key as FilterKey)}
        search={search}
        onSearchChange={setSearch}
      />

      {/* Loading indicators for REST sources */}
      {(activity.isLoading || debugEvents.isLoading || errors.isLoading) && (
        <div className={styles.loadingRow}>
          Loading
          {activity.isLoading ? ' activity' : ''}
          {debugEvents.isLoading ? ' events' : ''}
          {errors.isLoading ? ' errors' : ''}
          …
        </div>
      )}

      {/* REST fetch errors */}
      {activity.error && (
        <div className={styles.errorRow}>Activity: {activity.error instanceof Error ? activity.error.message : 'Error'}</div>
      )}
      {debugEvents.error && (
        <div className={styles.errorRow}>Debug events: {debugEvents.error instanceof Error ? debugEvents.error.message : 'Error'}</div>
      )}
      {errors.error && (
        <div className={styles.errorRow}>Errors: {errors.error instanceof Error ? errors.error.message : 'Error'}</div>
      )}

      {/* Timeline */}
      <div className={styles.timelineSection}>
        <div className={styles.sectionLabel}>Timeline</div>
        <EventFeed items={filteredItems} emptyMessage="No events match the current filter" />
      </div>
    </div>
  );
}
