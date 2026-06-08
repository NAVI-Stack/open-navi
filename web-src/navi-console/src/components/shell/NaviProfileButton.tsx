import { Button, Tooltip, TooltipTrigger } from 'react-aria-components';
import { Sparkles } from 'lucide-react';
import clsx from 'clsx';
import type { NaviPresence, StatusResponse } from '@/types/api';
import type { ConnectionState } from '@/types/events';
import styles from './NaviProfileButton.module.css';

export type NaviProfileStatus =
  | 'active'
  | 'idle'
  | 'dreaming'
  | 'working'
  | 'busy'
  | 'offline'
  | 'needs_attention'
  | 'wants_attention';

export const NAVI_PROFILE_STATUSES: NaviProfileStatus[] = [
  'active',
  'idle',
  'dreaming',
  'working',
  'busy',
  'offline',
  'needs_attention',
  'wants_attention',
];

export const NAVI_PROFILE_STATUS_LABELS: Record<NaviProfileStatus, string> = {
  active: 'Active',
  idle: 'Idle',
  dreaming: 'Dreaming',
  working: 'Working',
  busy: 'Busy',
  offline: 'Offline',
  needs_attention: 'Needs attention',
  wants_attention: 'Wants attention',
};

export const NAVI_PROFILE_STATUS_DESCRIPTIONS: Record<NaviProfileStatus, string> = {
  active: 'Ready and responsive',
  idle: 'Calm standby',
  dreaming: 'Background processing',
  working: 'Working and interruptible',
  busy: 'Busy and committed',
  offline: 'Unavailable',
  needs_attention: 'User action needed',
  wants_attention: 'Proposal available',
};

const naviProfileStatusSet = new Set<string>(NAVI_PROFILE_STATUSES);

const offlineConnectionStates = new Set<ConnectionState>(['disconnected', 'error', 'reconnecting']);

export function resolveNaviProfileStatus(
  naviPresence?: NaviPresence,
  systemStatus?: StatusResponse,
  connectionState?: ConnectionState,
  localWorking?: boolean,
): NaviProfileStatus {
  if (localWorking) {
    return 'working';
  }

  // Presence payload is authoritative when available.
  const publicStatus = naviPresence?.payload.public_status;
  const internalStatus = naviPresence?.payload.internal_status;

  if (internalStatus === 'processing' || internalStatus === 'tool_executing') {
    return 'working';
  }

  if (typeof publicStatus === 'string' && naviProfileStatusSet.has(publicStatus)) {
    return publicStatus as NaviProfileStatus;
  }

  // Fallback: governor tripped → surface this to the user.
  if (systemStatus?.governor.tripped) {
    return 'needs_attention';
  }

  // Fallback: WebSocket offline signals when HTTP still works.
  if (connectionState !== undefined && offlineConnectionStates.has(connectionState)) {
    return 'offline';
  }

  return 'idle';
}

export interface NaviProfileButtonProps {
  status: NaviProfileStatus;
  detail?: string;
  size?: 'rail' | 'demo' | 'lg';
  actionLabel?: string;
  onPress?: () => void;
  className?: string;
}

export function NaviProfileButton({
  status,
  detail,
  size = 'rail',
  actionLabel,
  onPress,
  className,
}: NaviProfileButtonProps) {
  const label = NAVI_PROFILE_STATUS_LABELS[status];
  const description = detail || NAVI_PROFILE_STATUS_DESCRIPTIONS[status];
  const ariaLabel = actionLabel ? `${actionLabel}. NAVI is ${label}.` : `NAVI profile, ${label}.`;

  return (
    <TooltipTrigger delay={250}>
      <Button
        className={clsx(styles.button, className)}
        data-status={status}
        data-size={size}
        onPress={onPress}
        aria-label={ariaLabel}
      >
        <span className={styles.baseRing} aria-hidden="true" />
        {status === 'working' || status === 'busy' ? (
          <span className={styles.sweepRing} aria-hidden="true">
            <span className={styles.sweepFill} />
            <span className={styles.sweepMask} />
          </span>
        ) : (
          <span className={styles.statusRing} aria-hidden="true" />
        )}
        <span className={styles.avatar} aria-hidden="true">
          <Sparkles className={styles.icon} strokeWidth={2.2} />
        </span>
      </Button>
      <Tooltip placement="right" className="navi-tooltip">
        <span className={styles.tooltipTitle}>NAVI: {label}</span>
        <span className={styles.tooltipDetail}>{description}</span>
      </Tooltip>
    </TooltipTrigger>
  );
}
