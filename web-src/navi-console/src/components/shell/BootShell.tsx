import { Button } from 'react-aria-components';
import { Loader2, WifiOff } from 'lucide-react';
import styles from './BootShell.module.css';

interface BootShellProps {
  state: 'checking' | 'offline';
  onRetry?: () => void;
}

export function BootShell({ state, onRetry }: BootShellProps) {
  return (
    <div className={styles.root}>
      <div className={styles.brand}>
        <span className={styles.mark}>NAVI</span>
        <span className={styles.sub}>Console</span>
      </div>

      {state === 'checking' ? (
        <div className={styles.status}>
          <Loader2 size={16} className={styles.spinner} aria-hidden />
          <span>Checking local console…</span>
        </div>
      ) : (
        <div className={styles.offline}>
          <div className={styles.offlineIcon}>
            <WifiOff size={20} aria-hidden />
          </div>
          <h1 className={styles.offlineTitle}>Can’t reach NaviD</h1>
          <p className={styles.offlineDesc}>
            The console couldn’t connect to the NAVI backend. Make sure NaviD is running, then retry.
          </p>
          <div className={styles.offlineActions}>
            {onRetry && (
              <Button className="navi-button navi-button-primary" onPress={onRetry}>
                Retry
              </Button>
            )}
            <a className="navi-button" href="/onboarding">
              Open setup
            </a>
          </div>
        </div>
      )}
    </div>
  );
}
