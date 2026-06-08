import clsx from 'clsx';
import {
  Cloud,
  HardDrive,
  Info,
  RotateCw,
  Sparkles,
  Star,
  Zap,
  Eye,
  Brain,
  Wrench,
  Image,
  FileText,
  Code2,
} from 'lucide-react';

import type { BrowserModel, ModelCapability } from './types';
import styles from './ModelRow.module.css';
import browserStyles from './ModelBrowser.module.css';

// ─── Capability icon map ───────────────────────────────────────────────────────

const CAP_ICONS: Record<ModelCapability, React.ElementType> = {
  fast: Zap,
  vision: Eye,
  reasoning: Brain,
  tools: Wrench,
  image: Image,
  pdf: FileText,
  code: Code2,
};

const CAP_COLOR_MAP: Record<ModelCapability, string> = {
  fast: browserStyles.capFast,
  vision: browserStyles.capVision,
  reasoning: browserStyles.capReasoning,
  tools: browserStyles.capTools,
  image: browserStyles.capImage,
  pdf: browserStyles.capPdf,
  code: browserStyles.capCode,
};

function CapIcon({ cap }: { cap: ModelCapability }) {
  const Icon = CAP_ICONS[cap];
  return (
    <span
      className={clsx(browserStyles.capDot, CAP_COLOR_MAP[cap])}
      title={cap}
      aria-label={cap}
    >
      <Icon size={10} />
    </span>
  );
}

// ─── Execution kind icon ───────────────────────────────────────────────────────

function KindIcon({ type }: { type: BrowserModel['executionType'] }) {
  if (type === 'local') return <HardDrive size={14} />;
  if (type === 'auto') return <Sparkles size={14} />;
  if (type === 'cloud') return <Cloud size={14} />;
  return <RotateCw size={14} />;
}

// ─── ModelRow ─────────────────────────────────────────────────────────────────

interface ModelRowProps {
  model: BrowserModel;
  selected: boolean;
  isFavorite: boolean;
  showProvider?: boolean;
  onSelect: (id: string) => void;
  onInfo: (id: string) => void;
  onToggleFavorite: (id: string) => void;
}

export function ModelRow({
  model,
  selected,
  isFavorite,
  showProvider,
  onSelect,
  onInfo,
  onToggleFavorite,
}: ModelRowProps) {
  const visibleCaps = model.capabilities.slice(0, 4);

  const speedHint =
    model.speedScore >= 80 ? '⚡ fast' :
    model.speedScore <= 30 ? '🐢 slow' :
    undefined;

  function handleRowClick() {
    onSelect(model.id);
  }

  function handleRowKey(e: React.KeyboardEvent) {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onSelect(model.id);
    }
  }

  function handleInfo(e: React.MouseEvent) {
    e.stopPropagation();
    onInfo(model.id);
  }

  function handleStar(e: React.MouseEvent) {
    e.stopPropagation();
    onToggleFavorite(model.id);
  }

  function handleInfoKey(e: React.KeyboardEvent) {
    if (e.key === 'Enter' || e.key === ' ') {
      e.stopPropagation();
      e.preventDefault();
      onInfo(model.id);
    }
  }

  function handleStarKey(e: React.KeyboardEvent) {
    if (e.key === 'Enter' || e.key === ' ') {
      e.stopPropagation();
      e.preventDefault();
      onToggleFavorite(model.id);
    }
  }

  return (
    <div
      role="option"
      aria-selected={selected}
      tabIndex={0}
      className={clsx(styles.row, selected && styles.rowSelected)}
      onClick={handleRowClick}
      onKeyDown={handleRowKey}
    >
      {/* Provider / execution icon */}
      <span className={styles.kindIcon} aria-hidden>
        <KindIcon type={model.executionType} />
      </span>

      {/* Name + badges */}
      <div className={styles.nameBlock}>
        <div className={styles.nameRow}>
          <span className={styles.name}>{model.displayName}</span>
          {speedHint && (
            <span className={styles.scoreHint}>{speedHint}</span>
          )}
        </div>

        {/* Provider sub-label shown in multi-provider views (All/Favorites) */}
        {showProvider && !model.isNaviAuto && (
          <div className={styles.providerSub}>{model.providerName}</div>
        )}

        {/* Badges: Auto + Legacy only — Cloud/Local redundant with left icon */}
        {(model.isNaviAuto || model.isLegacy) && (
          <div className={styles.badges}>
            {model.isNaviAuto && (
              <span className={clsx(styles.badge, styles.badgeAuto)}>
                <Sparkles size={9} /> Auto
              </span>
            )}
            {model.isLegacy && (
              <span className={clsx(styles.badge, styles.badgeLegacy)}>Legacy</span>
            )}
          </div>
        )}
      </div>

      {/* Right: capability icons + action buttons */}
      <div className={styles.actions}>
        {visibleCaps.length > 0 && (
          <div className={styles.capIcons}>
            {visibleCaps.map((cap) => (
              <CapIcon key={cap} cap={cap} />
            ))}
          </div>
        )}

        <button
          type="button"
          className={styles.iconBtn}
          onClick={handleInfo}
          onKeyDown={handleInfoKey}
          aria-label={`View details for ${model.displayName}`}
          title="Model details"
        >
          <Info size={12} />
        </button>

        <button
          type="button"
          className={clsx(styles.iconBtn, isFavorite && styles.starActive)}
          onClick={handleStar}
          onKeyDown={handleStarKey}
          aria-label={isFavorite ? `Remove ${model.displayName} from favorites` : `Add ${model.displayName} to favorites`}
          title={isFavorite ? 'Remove from favorites' : 'Add to favorites'}
        >
          <Star size={12} fill={isFavorite ? 'currentColor' : 'none'} />
        </button>
      </div>
    </div>
  );
}
