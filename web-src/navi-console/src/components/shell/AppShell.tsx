import { createContext, useContext, useState, useCallback, useEffect } from 'react';
import { LeftRail } from './LeftRail';
import { TopBar } from './TopBar';
import { RightInspector } from '../inspector/RightInspector';
import { PageRouter } from './PageRouter';
import { resolveNaviProfileStatus } from './NaviProfileButton';
import { useRoute } from '@/app/router';
import { useSystemStatus, useAgentStatus } from '@/api/status';
import { useNaviPresence } from '@/api/config';
import { useLiveEvents } from '@/hooks/useLiveEvents';
import { LiveEventsContext } from '@/hooks/LiveEventsContext';
import { isDraftChatId } from '@/lib/newChat';
import styles from './AppShell.module.css';
import clsx from 'clsx';

export interface LocalWorkingState {
  localWorking: boolean;
  setLocalWorking: (v: boolean) => void;
}

export const LocalWorkingContext = createContext<LocalWorkingState>({
  localWorking: false,
  setLocalWorking: () => {},
});

export function useLocalWorking() {
  return useContext(LocalWorkingContext);
}

interface InspectorState {
  isOpen: boolean;
  toggle: () => void;
  open: () => void;
  close: () => void;
}

const InspectorContext = createContext<InspectorState>({
  isOpen: false,
  toggle: () => {},
  open: () => {},
  close: () => {},
});

export function useInspector() {
  return useContext(InspectorContext);
}

interface RailState {
  collapsed: boolean;
  toggle: () => void;
  setCollapsed: (v: boolean) => void;
  /** Collapse without writing to localStorage — for temporary layout overrides. */
  silentCollapse: () => void;
}

const RailContext = createContext<RailState>({
  collapsed: false,
  toggle: () => {},
  setCollapsed: () => {},
  silentCollapse: () => {},
});

export function useRail() {
  return useContext(RailContext);
}

const INSPECTOR_KEY = 'navi-console-inspector-open';
const RAIL_COLLAPSED_KEY = 'navi-console-rail-collapsed';

function readInspectorState(): boolean {
  try {
    return window.localStorage.getItem(INSPECTOR_KEY) === 'true';
  } catch {
    return false;
  }
}

function storeInspectorState(open: boolean) {
  try {
    window.localStorage.setItem(INSPECTOR_KEY, String(open));
  } catch {}
}

function readRailCollapsedState(): boolean {
  try {
    return window.localStorage.getItem(RAIL_COLLAPSED_KEY) === 'true';
  } catch {
    return false;
  }
}

function storeRailCollapsedState(collapsed: boolean) {
  try {
    window.localStorage.setItem(RAIL_COLLAPSED_KEY, String(collapsed));
  } catch {}
}

export function AppShell() {
  const [inspectorOpen, setInspectorOpen] = useState(readInspectorState);
  const [railCollapsed, setRailCollapsed] = useState(readRailCollapsedState);
  const [localWorking, setLocalWorking] = useState(false);
  const route = useRoute();
  const status = useSystemStatus();
  const agent = useAgentStatus();
  const naviPresence = useNaviPresence();
  const activeChatId = route.params.chatId;
  const live = useLiveEvents({ chatId: isDraftChatId(activeChatId) ? undefined : activeChatId, enabled: true });
  const isBackendOffline = naviPresence.isError && status.isError;
  const naviProfileStatus = isBackendOffline
    ? 'offline'
    : resolveNaviProfileStatus(naviPresence.data, status.data, live.connectionState, localWorking);
  const naviPresencePayload = naviPresence.data?.payload;
  const naviProfileDetail = localWorking
    ? 'Initiating run...'
    : (naviPresencePayload?.status_text ??
       naviPresencePayload?.current_detail ??
       naviPresencePayload?.internal_status ??
       agent.data?.current_detail ??
       agent.data?.state ??
       (naviPresence.error ? 'NAVI presence unavailable' : undefined));

  const toggle = useCallback(() => {
    setInspectorOpen(prev => {
      const next = !prev;
      storeInspectorState(next);
      return next;
    });
  }, []);

  const inspectorState: InspectorState = {
    isOpen: inspectorOpen,
    toggle,
    open: () => { setInspectorOpen(true); storeInspectorState(true); },
    close: () => { setInspectorOpen(false); storeInspectorState(false); },
  };

  const setStoredRailCollapsed = useCallback((collapsed: boolean) => {
    setRailCollapsed(collapsed);
    storeRailCollapsedState(collapsed);
  }, []);

  const toggleRailCollapsed = useCallback(() => {
    setRailCollapsed(prev => {
      const next = !prev;
      storeRailCollapsedState(next);
      return next;
    });
  }, []);

  const expandRail = useCallback(() => {
    setStoredRailCollapsed(false);
  }, [setStoredRailCollapsed]);

  const silentCollapse = useCallback(() => {
    setRailCollapsed(true);
  }, []);

  const railState: RailState = {
    collapsed: railCollapsed,
    toggle: toggleRailCollapsed,
    setCollapsed: setStoredRailCollapsed,
    silentCollapse,
  };

  useEffect(() => {
    const handleResize = () => {
      if (window.innerWidth < 768) {
        setStoredRailCollapsed(true);
      }
    };
    handleResize();
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, [setStoredRailCollapsed]);

  return (
    <LiveEventsContext.Provider value={live}>
      <InspectorContext.Provider value={inspectorState}>
        <RailContext.Provider value={railState}>
          <LocalWorkingContext.Provider value={{ localWorking, setLocalWorking }}>
            <div className={clsx(styles.shell, railCollapsed && styles.railCollapsed, inspectorOpen && styles.inspectorOpen)}>
              <LeftRail
                collapsed={railCollapsed}
                onToggleCollapsed={toggleRailCollapsed}
                onExpand={expandRail}
                profileStatus={naviProfileStatus}
                profileDetail={naviProfileDetail}
              />
              <div className={styles.mainArea}>
                <TopBar
                  version={status.data?.gateway.version}
                />
                <main className={styles.content}>
                  <PageRouter />
                </main>
              </div>
              {inspectorOpen && <RightInspector />}
            </div>
          </LocalWorkingContext.Provider>
        </RailContext.Provider>
      </InspectorContext.Provider>
    </LiveEventsContext.Provider>
  );
}
