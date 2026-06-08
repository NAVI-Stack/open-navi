import React, { useEffect, useRef, useState, useMemo } from 'react';
import clsx from 'clsx';
import {
  Check,
  ChevronRight,
  Cloud,
  Globe,
  HardDrive,
  LayoutGrid,
  Loader2,
  Plus,
  Search,
  Settings,
  SlidersHorizontal,
  Sparkles,
  Star,
  X,
} from 'lucide-react';
import type { BrowserModel, BrowserProvider, ModelCapability, SortMode } from './types';
import { CAPABILITY_META, SORT_OPTIONS } from './types';
import { ModelRow } from './ModelRow';
import { ModelDetailsPanel } from './ModelDetailsPanel';
import { AddModelModal } from './AddModelModal';
import styles from './ModelBrowser.module.css';

// ─── Helpers ──────────────────────────────────────────────────────────────────

function sortModels(list: BrowserModel[], mode: SortMode, favoriteIds: string[]): BrowserModel[] {
  const sorted = [...list];

  sorted.sort((a, b) => {
    // NAVI Auto always pins to top for recommended/default sorts
    if (a.isNaviAuto && mode === 'recommended') return -1;
    if (b.isNaviAuto && mode === 'recommended') return 1;

    switch (mode) {
      case 'fastest':
        return b.speedScore - a.speedScore;
      case 'cheapest':
        return b.costScore - a.costScore;
      case 'reasoning':
        return b.reasoningScore - a.reasoningScore;
      case 'coding':
        return b.codingScore - a.codingScore;
      case 'local':
        if (a.executionType === 'local' && b.executionType !== 'local') return -1;
        if (b.executionType === 'local' && a.executionType !== 'local') return 1;
        return b.costScore - a.costScore;
      case 'recent':
        // No date metadata available — fall back to display name alphabetical
        return a.displayName.localeCompare(b.displayName);
      case 'recommended':
      default: {
        const aRec = a.isRecommended ? 1 : 0;
        const bRec = b.isRecommended ? 1 : 0;
        if (aRec !== bRec) return bRec - aRec;
        const aFav = favoriteIds.includes(a.id) ? 1 : 0;
        const bFav = favoriteIds.includes(b.id) ? 1 : 0;
        if (aFav !== bFav) return bFav - aFav;
        return b.agenticScore - a.agenticScore;
      }
    }
  });

  return sorted;
}

// ─── Provider rail button ─────────────────────────────────────────────────────

function ProviderRailBtn({
  label,
  icon,
  status,
  isActive,
  onClick,
}: {
  providerKey?: string;
  label: string;
  icon: React.ReactNode;
  status?: BrowserProvider['status'];
  isActive: boolean;
  onClick: () => void;
}) {
  const dotClass =
    status === 'connected' ? styles.dotConnected :
    status === 'offline' ? styles.dotOffline :
    status === 'unavailable' ? styles.dotUnavailable :
    styles.dotUnknown;

  const stateClass =
    status === 'connected' ? styles.railBtnConnected :
    status === 'offline' ? styles.railBtnOffline :
    status === 'unavailable' ? styles.railBtnUnavailable :
    '';

  return (
    <button
      type="button"
      title={`${label}${status ? ` — ${status.replace('_', ' ')}` : ''}`}
      aria-pressed={isActive}
      aria-label={label}
      className={clsx(
        styles.railBtn,
        isActive && styles.railBtnActive,
        !isActive && stateClass,
      )}
      onClick={onClick}
    >
      {icon}
      {status && status !== 'unknown' && !isActive && (
        <span className={clsx(styles.railStatusDot, dotClass)} aria-hidden />
      )}
    </button>
  );
}

// ─── ModelBrowser ─────────────────────────────────────────────────────────────

interface ModelBrowserProps {
  models: BrowserModel[];
  providers: BrowserProvider[];
  selectedId: string;
  favoriteIds: string[];
  isLoading?: boolean;
  onSelect: (id: string) => void;
  onToggleFavorite: (id: string) => void;
  onClose: () => void;
  placement?: 'top' | 'bottom';
}

export function ModelBrowser({
  models,
  providers,
  selectedId,
  favoriteIds,
  isLoading = false,
  onSelect,
  onToggleFavorite,
  onClose,
  placement = 'top',
}: ModelBrowserProps) {
  const [query, setQuery] = useState('');
  const [activeProvider, setActiveProvider] = useState<string>('all');
  const [activeCapabilities, setActiveCapabilities] = useState<ModelCapability[]>([]);
  const [sortMode, setSortMode] = useState<SortMode>('recommended');
  const [legacyMerged, setLegacyMerged] = useState(false);
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [detailModelId, setDetailModelId] = useState<string | null>(null);
  const [addModalOpen, setAddModalOpen] = useState(false);

  const panelRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);

  // Focus search on open
  useEffect(() => {
    searchRef.current?.focus();
  }, []);

  // Escape to close (addModalOpen takes priority — let the modal handle its own Escape)
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        if (addModalOpen) return;
        if (detailModelId) {
          setDetailModelId(null);
        } else if (filtersOpen) {
          setFiltersOpen(false);
        } else {
          onClose();
        }
      }
    }
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [onClose, detailModelId, filtersOpen, addModalOpen]);

  // Close filter menu when clicking outside
  useEffect(() => {
    if (!filtersOpen) return;
    function onPointerDown(e: PointerEvent) {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) return;
      const target = e.target as HTMLElement;
      if (!target.closest('[data-filter-menu]') && !target.closest('[data-filter-btn]')) {
        setFiltersOpen(false);
      }
    }
    document.addEventListener('pointerdown', onPointerDown);
    return () => document.removeEventListener('pointerdown', onPointerDown);
  }, [filtersOpen]);

  // Filtered + sorted models
  const { primaryModels, legacyModels } = useMemo(() => {
    const q = query.trim().toLowerCase();

    const filtered = models.filter((m) => {
      if (q) {
        const searchable = [
          m.displayName,
          m.modelId,
          m.providerKey,
          m.providerName,
          ...m.tags,
          ...m.capabilities,
          m.executionType,
        ]
          .join(' ')
          .toLowerCase();
        if (!searchable.includes(q)) return false;
      } else {
        if (activeProvider === 'all') {
          // No filter — show everything
        } else if (activeProvider === 'favorites') {
          if (!favoriteIds.includes(m.id)) return false;
        } else if (activeProvider === 'navi') {
          if (m.providerKey !== 'navi') return false;
        } else {
          if (m.providerKey !== activeProvider) return false;
        }
      }

      if (
        activeCapabilities.length > 0 &&
        !activeCapabilities.every((cap) => m.capabilities.includes(cap))
      ) {
        return false;
      }

      return true;
    });

    const sorted = sortModels(filtered, sortMode, favoriteIds);

    return {
      primaryModels: sorted.filter((m) => !m.isLegacy),
      legacyModels: sorted.filter((m) => m.isLegacy),
    };
  }, [models, query, activeProvider, activeCapabilities, sortMode, favoriteIds]);

  const visibleModels = legacyMerged
    ? [...primaryModels, ...legacyModels]
    : primaryModels;

  const totalVisible = visibleModels.length;
  const showCount = !isLoading || totalVisible > 0;
  const filterCount = activeCapabilities.length;
  const currentSortLabel =
    SORT_OPTIONS.find((o) => o.id === sortMode)?.label ?? 'Recommended';

  const detailModel = detailModelId
    ? models.find((m) => m.id === detailModelId)
    : null;

  // Show provider name when multiple providers may be in the list simultaneously.
  // Always shown during search (results cross providers regardless of active tab).
  const showProvider = !!query || activeProvider === 'all' || activeProvider === 'favorites';

  function toggleCapability(cap: ModelCapability) {
    setActiveCapabilities((prev) =>
      prev.includes(cap) ? prev.filter((c) => c !== cap) : [...prev, cap],
    );
  }

  function clearFilters() {
    setActiveCapabilities([]);
    setSortMode('recommended');
  }

  // Filter providers to only those with available models
  const visibleProviders = useMemo(() => {
    const hasNavi = models.some((m) => m.providerKey === 'navi');
    const hasFavorites = models.some((m) => favoriteIds.includes(m.id));

    const list = [];
    if (models.length > 0) {
      list.push({ key: 'all', label: 'All', kind: 'virtual' as const });
    }
    if (hasNavi) {
      list.push({ key: 'navi', label: 'NAVI', kind: 'virtual' as const });
    }
    if (hasFavorites) {
      list.push({ key: 'favorites', label: 'Favorites', kind: 'virtual' as const });
    }

    const providersWithModels = providers.filter((p) =>
      models.some((m) => m.providerKey === p.key),
    );

    list.push(
      ...providersWithModels.map((p) => ({
        key: p.key,
        label: p.displayName,
        kind: p.kind,
        status: p.status,
      })),
    );

    return list;
  }, [models, providers, favoriteIds]);

  // If the active provider tab is hidden, fallback to the first visible one
  useEffect(() => {
    if (
      visibleProviders.length > 0 &&
      !visibleProviders.some((p) => p.key === activeProvider)
    ) {
      setActiveProvider(visibleProviders[0].key);
    }
  }, [visibleProviders, activeProvider]);

  return (
    <>
      {/* Transparent backdrop — absorbs pointer events outside the panel to close it */}
      <div
        className={styles.backdrop}
        onPointerDown={(e) => { e.stopPropagation(); onClose(); }}
        aria-hidden
      />
      <div
        ref={panelRef}
        className={clsx(
          styles.panel,
          placement === 'bottom' ? styles.panelBottom : styles.panelTop
        )}
        role="listbox"
        aria-label="Select model"
      >
        {/* ─── Provider rail ─────────────────────────────────────────────── */}
        <aside className={styles.rail} aria-label="Filter by provider">
          {visibleProviders.map((p, i) => {
            // Separator between virtual tabs (all/navi/favorites) and real providers
            const isFirstReal = p.kind !== 'virtual' && i > 0 && visibleProviders[i - 1].kind === 'virtual';

            let icon: React.ReactNode;
            if (p.key === 'all') {
              icon = <LayoutGrid size={15} />;
            } else if (p.key === 'navi') {
              icon = <Sparkles size={15} />;
            } else if (p.key === 'favorites') {
              icon = <Star size={15} />;
            } else if (p.kind === 'local') {
              icon = <HardDrive size={15} />;
            } else if (p.kind === 'proxy') {
              icon = <Globe size={15} />;
            } else {
              icon = <Cloud size={15} />;
            }

            return (
              <React.Fragment key={p.key}>
                {isFirstReal && <div className={styles.railSeparator} aria-hidden />}
                <ProviderRailBtn
                  label={p.label}
                  icon={icon}
                  status={'status' in p ? p.status : undefined}
                  isActive={activeProvider === p.key}
                  onClick={() => {
                    setActiveProvider(p.key);
                    setQuery('');
                    setLegacyMerged(false);
                  }}
                />
              </React.Fragment>
            );
          })}
        </aside>

        {/* ─── Right content ─────────────────────────────────────────────── */}
        <div className={styles.content}>
          {/* Top bar */}
          <div className={styles.topBar}>
            <span className={styles.topBarLabel}>
              {showCount ? (
                <>
                  {totalVisible} model{totalVisible !== 1 ? 's' : ''}
                  {sortMode !== 'recommended' && (
                    <> · <span className={styles.topBarSort}>{currentSortLabel}</span></>
                  )}
                </>
              ) : '…'}
            </span>
            <div className={styles.topBarActions}>
              <button
                type="button"
                className={styles.topBarBtn}
                onClick={() => setAddModalOpen(true)}
                aria-label="Add provider or model"
              >
                <Plus size={12} /> Providers
              </button>
              <button
                type="button"
                className={styles.topBarBtn}
                aria-label="LLM settings"
                title="Open LLM settings"
                onClick={() => {
                  onClose();
                  window.history.pushState(null, '', '/settings');
                  window.dispatchEvent(new PopStateEvent('popstate'));
                }}
              >
                <Settings size={12} />
              </button>
            </div>
          </div>

          {/* Search bar */}
          <div className={styles.searchBar}>
            <Search size={14} className={styles.searchIcon} aria-hidden />
            <input
              ref={searchRef}
              type="search"
              className={styles.searchInput}
              placeholder="Search models…"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Escape') {
                  if (query) {
                    setQuery('');
                    e.stopPropagation();
                  }
                }
              }}
              aria-label="Search models"
            />
            {query && (
              <button
                type="button"
                className={styles.searchClearBtn}
                onClick={() => setQuery('')}
                aria-label="Clear search"
              >
                <X size={11} />
              </button>
            )}
            <button
              type="button"
              data-filter-btn
              className={clsx(
                styles.filterBtn,
                (filtersOpen || filterCount > 0) && styles.filterBtnActive,
              )}
              onClick={() => setFiltersOpen((v) => !v)}
              aria-expanded={filtersOpen}
              aria-label={`Filters and sort — ${currentSortLabel}`}
              title={`Sort: ${currentSortLabel}`}
            >
              <SlidersHorizontal size={13} />
              {filterCount > 0 && (
                <span className={styles.filterCount} aria-label={`${filterCount} active filters`}>
                  {filterCount}
                </span>
              )}
            </button>
          </div>

          {/* Filter/sort menu */}
          {filtersOpen && (
            <div className={styles.filterMenu} data-filter-menu role="menu">
              <div className={styles.filterMenuSection}>Capabilities</div>
              {(Object.keys(CAPABILITY_META) as ModelCapability[]).map((cap) => {
                const meta = CAPABILITY_META[cap];
                const isActive = activeCapabilities.includes(cap);
                return (
                  <button
                    key={cap}
                    type="button"
                    role="menuitemcheckbox"
                    aria-checked={isActive}
                    className={clsx(
                      styles.filterCapBtn,
                      isActive && styles.filterCapBtnActive,
                    )}
                    onClick={() => toggleCapability(cap)}
                  >
                    {meta.label}
                    {isActive && <Check size={12} className={styles.filterCapCheck} />}
                  </button>
                );
              })}

              <hr className={styles.filterMenuDivider} />
              <div className={styles.filterMenuSection}>Sort</div>
              <div className={styles.sortGrid}>
                {SORT_OPTIONS.map((opt) => (
                  <button
                    key={opt.id}
                    type="button"
                    role="menuitemradio"
                    aria-checked={sortMode === opt.id}
                    className={clsx(
                      styles.sortBtn,
                      sortMode === opt.id && styles.sortBtnActive,
                    )}
                    onClick={() => setSortMode(opt.id)}
                  >
                    {opt.label}
                  </button>
                ))}
              </div>

              <hr className={styles.filterMenuDivider} />
              <button
                type="button"
                className={styles.clearFiltersBtn}
                onClick={() => {
                  clearFilters();
                  setFiltersOpen(false);
                }}
              >
                Clear filters
              </button>
            </div>
          )}

          {/* Model list */}
          <div
            className={styles.modelList}
            role="group"
            aria-label="Available models"
          >
            {isLoading && totalVisible === 0 && (
              <div className={styles.loadingState}>
                <Loader2 size={15} className={styles.loadingSpinner} aria-hidden />
                Loading models…
              </div>
            )}

            {visibleModels.map((model) => (
              <ModelRow
                key={model.id}
                model={model}
                selected={selectedId === model.id}
                isFavorite={favoriteIds.includes(model.id)}
                showProvider={showProvider}
                onSelect={onSelect}
                onInfo={setDetailModelId}
                onToggleFavorite={onToggleFavorite}
              />
            ))}

            {legacyModels.length > 0 && !legacyMerged && (
              <button
                type="button"
                className={styles.legacyReveal}
                onClick={() => setLegacyMerged(true)}
              >
                <ChevronRight size={13} aria-hidden />
                {legacyModels.length} legacy model
                {legacyModels.length !== 1 ? 's' : ''}
              </button>
            )}

            {/* NAVI routing modes stub — dev-only until routing modes ship */}
            {import.meta.env.DEV && activeProvider === 'navi' && !query && (
              <div className={styles.naviModesSection}>
                <div className={styles.naviModesSectionTitle}>Routing modes</div>
                <div className={styles.naviModesComingSoon}>
                  Efficiency · Power · Adaptive — coming soon
                </div>
              </div>
            )}

            {!isLoading && primaryModels.length === 0 && legacyModels.length === 0 && (
              <div className={styles.emptyState}>
                {providers.length === 0 && !query ? (
                  <>
                    No providers configured.{' '}
                    <button
                      type="button"
                      className={styles.emptyStateAction}
                      onClick={() => setAddModalOpen(true)}
                    >
                      Add a provider
                    </button>
                    {' '}to get started.
                  </>
                ) : query ? (
                  'No models match your search.'
                ) : activeCapabilities.length > 0 ? (
                  'No models match the active capability filters.'
                ) : (
                  'No models available.'
                )}
              </div>
            )}
          </div>

          {/* Detail panel overlay */}
          {detailModel && (
            <ModelDetailsPanel
              model={detailModel}
              onClose={() => setDetailModelId(null)}
            />
          )}
        </div>
      </div>

      {/* Add modal (rendered outside panel to avoid stacking context issues) */}
      {addModalOpen && (
        <AddModelModal onClose={() => setAddModalOpen(false)} />
      )}
    </>
  );
}
