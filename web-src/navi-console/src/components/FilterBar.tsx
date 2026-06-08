import clsx from 'clsx';
import { Search } from 'lucide-react';
import styles from './FilterBar.module.css';

export interface FilterTab {
  key: string;
  label: string;
  count?: number;
}

interface FilterBarProps {
  tabs: FilterTab[];
  activeTab: string;
  onTabChange: (key: string) => void;
  search: string;
  onSearchChange: (val: string) => void;
}

export function FilterBar({ tabs, activeTab, onTabChange, search, onSearchChange }: FilterBarProps) {
  return (
    <div className={styles.bar}>
      <div className={styles.tabs} role="tablist">
        {tabs.map((tab) => (
          <button
            key={tab.key}
            role="tab"
            aria-selected={activeTab === tab.key}
            className={clsx(styles.tab, activeTab === tab.key && styles.active)}
            onClick={() => onTabChange(tab.key)}
          >
            {tab.label}
            {tab.count !== undefined && tab.count > 0 && (
              <span className={styles.count}>{tab.count}</span>
            )}
          </button>
        ))}
      </div>
      <div className={styles.searchWrap}>
        <Search size={14} className={styles.searchIcon} />
        <input
          type="text"
          className={styles.searchInput}
          placeholder="Filter…"
          value={search}
          onChange={(e) => onSearchChange(e.target.value)}
          aria-label="Filter events"
        />
      </div>
    </div>
  );
}
