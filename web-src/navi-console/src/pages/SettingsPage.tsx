import { useState } from 'react';
import { Config } from './Config';
import { Appearance } from './Appearance';
import styles from './SettingsPage.module.css';
import { Settings, Palette, Activity, HeartHandshake, SlidersHorizontal } from 'lucide-react';
import { Workspaces } from './Workspaces';

export function SettingsPage() {
  const [activeTab, setActiveTab] = useState<'system' | 'appearance' | 'workspaces' | 'relationship'>('system');

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.titleInfo}>
          <h1 className={styles.title}>Settings</h1>
        </div>
        <p className={styles.description}>
          Manage NAVI core configuration, workspace permissions, and console appearance.
        </p>

        <div className={styles.tabSwitcher}>
          <button 
            className={`navi-tab ${activeTab === 'system' ? styles.tabActive : ''}`}
            onClick={() => setActiveTab('system')}
            data-selected={activeTab === 'system' ? true : undefined}
          >
            <Settings size={14} className={styles.tabIcon} />
            System
          </button>
          <button 
            className={`navi-tab ${activeTab === 'appearance' ? styles.tabActive : ''}`}
            onClick={() => setActiveTab('appearance')}
            data-selected={activeTab === 'appearance' ? true : undefined}
          >
            <Palette size={14} className={styles.tabIcon} />
            Appearance
          </button>
          <button 
            className={`navi-tab ${activeTab === 'workspaces' ? styles.tabActive : ''}`}
            onClick={() => setActiveTab('workspaces')}
            data-selected={activeTab === 'workspaces' ? true : undefined}
          >
            <Activity size={14} className={styles.tabIcon} />
            Workspaces
          </button>
          <button
            className={`navi-tab ${activeTab === 'relationship' ? styles.tabActive : ''}`}
            onClick={() => setActiveTab('relationship')}
            data-selected={activeTab === 'relationship' ? true : undefined}
          >
            <HeartHandshake size={14} className={styles.tabIcon} />
            NAVI relationship
          </button>
        </div>
      </div>

      <div className={styles.content}>
        {activeTab === 'system' && <div className={styles.legacyWrapper}><Config /></div>}
        {activeTab === 'appearance' && <div className={styles.legacyWrapper}><Appearance /></div>}
        {activeTab === 'workspaces' && <div className={styles.legacyWrapper}><Workspaces /></div>}
        {activeTab === 'relationship' && (
          <section className={styles.relationshipPanel}>
            <div className={styles.relationshipHeader}>
              <HeartHandshake size={20} />
              <div>
                <h2>NAVI relationship</h2>
                <p>Adjust how NAVI addresses you, shows up, and asks before acting.</p>
              </div>
            </div>
            <a className={styles.relationshipAction} href="/ceremony?mode=adjust">
              <SlidersHorizontal size={15} />
              Adjust relationship choices
            </a>
          </section>
        )}
      </div>
    </div>
  );
}
