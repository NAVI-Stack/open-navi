import { useState } from 'react';
import { Debug } from './Debug';
import { Logs } from './Logs';
import styles from './DebugPage.module.css';
import { Activity, Terminal } from 'lucide-react';

export function DebugPage() {
  const [activeTab, setActiveTab] = useState<'inspector' | 'logs'>('inspector');

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.titleInfo}>
          <h1 className={styles.title}>System Debug</h1>
        </div>
        <p className={styles.description}>
          Low-level system state, event stream, and runtime metrics for operator diagnostics.
        </p>

        <div className={styles.tabSwitcher}>
          <button 
            className={`navi-tab ${activeTab === 'inspector' ? styles.tabActive : ''}`}
            onClick={() => setActiveTab('inspector')}
            data-selected={activeTab === 'inspector' ? true : undefined}
          >
            <Activity size={14} className={styles.tabIcon} />
            Inspector
          </button>
          <button 
            className={`navi-tab ${activeTab === 'logs' ? styles.tabActive : ''}`}
            onClick={() => setActiveTab('logs')}
            data-selected={activeTab === 'logs' ? true : undefined}
          >
            <Terminal size={14} className={styles.tabIcon} />
            Event Logs
          </button>
        </div>
      </div>

      <div className={styles.content}>
        {activeTab === 'inspector' && <div className={styles.legacyWrapper}><Debug /></div>}
        {activeTab === 'logs' && <div className={styles.legacyWrapper}><Logs /></div>}
      </div>
    </div>
  );
}
