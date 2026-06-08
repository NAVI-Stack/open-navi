import { useSkills, usePlugins } from '@/api/extensions';
import { useNavigate } from '@/app/router';
import { Card } from '@/components/ui/Card';
import { StatusBadge } from '@/components/ui/StatusBadge';
import { EmptyState } from '@/components/ui/EmptyState';
import { Blocks, Wrench, Search, ChevronRight, ArrowUpDown } from 'lucide-react';
import { useState } from 'react';
import styles from './PluginsPage.module.css';

type SortOption = 'name-asc' | 'name-desc' | 'date-newest' | 'date-oldest' | 'status-active' | 'status-inactive';

export function PluginsPage() {
  const { data: pluginsData, isLoading: pluginsLoading } = usePlugins();
  const { data: skillsData, isLoading: skillsLoading } = useSkills();
  const navigate = useNavigate();
  const [activeTab, setActiveTab] = useState<'plugins' | 'skills'>('plugins');
  const [search, setSearch] = useState('');
  const [sortBy, setSortBy] = useState<SortOption>('name-asc');

  if (pluginsLoading || skillsLoading) {
    return <div className={styles.loading}>Loading extensions...</div>;
  }

  const plugins = pluginsData?.plugins || [];
  const skills = skillsData?.items || [];

  const filteredPlugins = plugins.filter(p => 
    p.name?.toLowerCase().includes(search.toLowerCase()) || 
    p.id?.toLowerCase().includes(search.toLowerCase())
  );

  const filteredSkills = skills.filter(s => 
    s.name?.toLowerCase().includes(search.toLowerCase()) || 
    s.id?.toLowerCase().includes(search.toLowerCase())
  );

  const sortedPlugins = [...filteredPlugins].sort((a, b) => {
    if (sortBy === 'name-asc') {
      return (a.name || a.id).localeCompare(b.name || b.id);
    }
    if (sortBy === 'name-desc') {
      return (b.name || b.id).localeCompare(a.name || a.id);
    }
    if (sortBy === 'date-newest') {
      const da = a.date_added ? new Date(a.date_added).getTime() : 0;
      const db = b.date_added ? new Date(b.date_added).getTime() : 0;
      return db - da;
    }
    if (sortBy === 'date-oldest') {
      const da = a.date_added ? new Date(a.date_added).getTime() : 0;
      const db = b.date_added ? new Date(b.date_added).getTime() : 0;
      return da - db;
    }
    if (sortBy === 'status-active') {
      const aAct = a.active ? 1 : 0;
      const bAct = b.active ? 1 : 0;
      if (aAct !== bAct) return bAct - aAct;
      return (a.name || a.id).localeCompare(b.name || b.id);
    }
    if (sortBy === 'status-inactive') {
      const aAct = a.active ? 1 : 0;
      const bAct = b.active ? 1 : 0;
      if (aAct !== bAct) return aAct - bAct;
      return (a.name || a.id).localeCompare(b.name || b.id);
    }
    return 0;
  });

  const sortedSkills = [...filteredSkills].sort((a, b) => {
    if (sortBy === 'name-asc') {
      return (a.name || a.id).localeCompare(b.name || b.id);
    }
    if (sortBy === 'name-desc') {
      return (b.name || b.id).localeCompare(a.name || a.id);
    }
    if (sortBy === 'status-active') {
      const aVal = a.validation_status === 'valid' || a.enabled ? 1 : 0;
      const bVal = b.validation_status === 'valid' || b.enabled ? 1 : 0;
      if (aVal !== bVal) return bVal - aVal;
      return (a.name || a.id).localeCompare(b.name || b.id);
    }
    if (sortBy === 'status-inactive') {
      const aVal = a.validation_status === 'valid' || a.enabled ? 1 : 0;
      const bVal = b.validation_status === 'valid' || b.enabled ? 1 : 0;
      if (aVal !== bVal) return aVal - bVal;
      return (a.name || a.id).localeCompare(b.name || b.id);
    }
    return (a.name || a.id).localeCompare(b.name || b.id);
  });

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.titleInfo}>
          <h1 className={styles.title}>Extensions</h1>
        </div>
        <p className={styles.description}>
          Manage installed plugins and active skills within the agent environment.
        </p>

        <div className={styles.toolbar}>
          <div className={styles.toolbarLeft}>
            <div className="navi-searchfield" data-focused={undefined}>
              <Search size={16} className={styles.searchIcon} />
              <input 
                type="text" 
                placeholder="Search extensions..." 
                className="navi-searchfield-input"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
            </div>
            
            <div className={styles.sortContainer}>
              <ArrowUpDown size={16} className={styles.sortIcon} />
              <select
                value={sortBy}
                onChange={(e) => setSortBy(e.target.value as SortOption)}
                className={styles.sortSelect}
              >
                <option value="name-asc">Name (A-Z)</option>
                <option value="name-desc">Name (Z-A)</option>
                <option value="date-newest">Date Added (Newest)</option>
                <option value="date-oldest">Date Added (Oldest)</option>
                <option value="status-active">Status (Active First)</option>
                <option value="status-inactive">Status (Inactive First)</option>
              </select>
            </div>
          </div>
          
          <div className={styles.tabSwitcher}>
            <button 
              className={`navi-tab ${activeTab === 'plugins' ? styles.tabActive : ''}`}
              onClick={() => setActiveTab('plugins')}
              data-selected={activeTab === 'plugins' ? true : undefined}
            >
              Plugins ({plugins.length})
            </button>
            <button 
              className={`navi-tab ${activeTab === 'skills' ? styles.tabActive : ''}`}
              onClick={() => setActiveTab('skills')}
              data-selected={activeTab === 'skills' ? true : undefined}
            >
              Skills ({skills.length})
            </button>
          </div>
        </div>
      </div>

      <div className={styles.content}>
        {activeTab === 'plugins' && (
          <div className={styles.grid}>
            {sortedPlugins.length > 0 ? sortedPlugins.map(plugin => (
              <Card
                key={plugin.id}
                className={styles.card}
                hoverable
                onClick={() => navigate('/plugins/' + encodeURIComponent(plugin.id))}
              >
                <div className={styles.cardHeader}>
                  <div className={styles.cardTitleRow}>
                    <Blocks size={20} className={styles.cardIcon} />
                    <span className={styles.cardTitle}>{plugin.name || plugin.id}</span>
                  </div>
                  <div className={styles.cardTitleRow}>
                    <StatusBadge
                      label={plugin.active ? 'active' : 'inactive'}
                      variant={plugin.active ? 'running' : 'default'}
                    />
                    <ChevronRight size={16} className={styles.cardIcon} />
                  </div>
                </div>
                <div className={styles.cardBody}>
                  <p className={styles.cardDesc}>
                    {plugin.metadata?.description || 'No description available.'}
                  </p>
                  <div className={styles.cardMeta}>
                    <span className={styles.badge}>{plugin.version || 'v1.0.0'}</span>
                    {plugin.kind && <span className={styles.badge}>{plugin.kind}</span>}
                  </div>
                </div>
              </Card>
            )) : (
              <EmptyState 
                icon={<Blocks size={24} />} 
                title="No plugins found" 
                description="Try adjusting your search query." 
              />
            )}
          </div>
        )}

        {activeTab === 'skills' && (
          <div className={styles.grid}>
            {sortedSkills.length > 0 ? sortedSkills.map(skill => (
              <Card key={skill.id} className={styles.card}>
                <div className={styles.cardHeader}>
                  <div className={styles.cardTitleRow}>
                    <Wrench size={20} className={styles.cardIcon} />
                    <span className={styles.cardTitle}>{skill.name || skill.id}</span>
                  </div>
                  <StatusBadge 
                    label={skill.validation_status || (skill.enabled ? 'valid' : 'invalid')} 
                    variant={skill.validation_status === 'valid' ? 'completed' : 'failed'} 
                  />
                </div>
                <div className={styles.cardBody}>
                  <div className={styles.cardMeta}>
                    <span className={styles.badge}>Plugin: {skill.plugin_owner || 'unknown'}</span>
                    {skill.source && <span className={styles.badge}>{skill.source}</span>}
                  </div>
                  {skill.last_error && (
                    <div className={styles.errorBox}>
                      {String(skill.last_error)}
                    </div>
                  )}
                </div>
              </Card>
            )) : (
              <EmptyState 
                icon={<Wrench size={24} />} 
                title="No skills found" 
                description="Try adjusting your search query." 
              />
            )}
          </div>
        )}
      </div>
    </div>
  );
}
