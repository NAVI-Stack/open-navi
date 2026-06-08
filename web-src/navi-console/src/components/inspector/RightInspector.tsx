import { Tabs, TabList, Tab, TabPanel, Button } from 'react-aria-components';
import { X } from 'lucide-react';
import { useRoute } from '@/app/router';
import { useInspector } from '../shell/AppShell';
import { RuntimePanel } from './RuntimePanel';
import { RecentIntakePanel } from '@/components/intake/RecentIntakePanel';
import styles from './RightInspector.module.css';

export function RightInspector() {
  const { route, params } = useRoute();
  const inspector = useInspector();

  const renderTabs = () => {
    if (route === 'chats') {
      return (
        <Tabs className="navi-tabs">
          <TabList className="navi-tablist">
            <Tab id="runtime" className="navi-tab">Runtime</Tab>
            <Tab id="context" className="navi-tab">Context</Tab>
            <Tab id="debug" className="navi-tab">Debug</Tab>
          </TabList>
          <TabPanel id="runtime" className="navi-tabpanel">
            <RuntimePanel chatId={params.chatId} />
          </TabPanel>
          <TabPanel id="context" className="navi-tabpanel">
            {/* CIP P5: recent connector-sourced intake with provenance links. */}
            <RecentIntakePanel />
          </TabPanel>
          <TabPanel id="debug" className="navi-tabpanel">
            <div className={styles.placeholder}>Debug panel</div>
          </TabPanel>
        </Tabs>
      );
    }

    if (route === 'projects') {
      return (
        <Tabs className="navi-tabs">
          <TabList className="navi-tablist">
            <Tab id="readiness" className="navi-tab">Readiness</Tab>
            <Tab id="workspace" className="navi-tab">Workspace</Tab>
            <Tab id="chats" className="navi-tab">Chats</Tab>
          </TabList>
          <TabPanel id="readiness" className="navi-tabpanel">
            <div className={styles.placeholder}>Readiness panel</div>
          </TabPanel>
          <TabPanel id="workspace" className="navi-tabpanel">
            <div className={styles.placeholder}>Workspace panel</div>
          </TabPanel>
          <TabPanel id="chats" className="navi-tabpanel">
            <div className={styles.placeholder}>Chats panel</div>
          </TabPanel>
        </Tabs>
      );
    }

    if (route === 'runs') {
      return (
        <Tabs className="navi-tabs">
          <TabList className="navi-tablist">
            <Tab id="phase" className="navi-tab">Phase</Tab>
            <Tab id="tools" className="navi-tab">Tool Calls</Tab>
            <Tab id="errors" className="navi-tab">Errors</Tab>
          </TabList>
          <TabPanel id="phase" className="navi-tabpanel">
            <div className={styles.placeholder}>Phase panel</div>
          </TabPanel>
          <TabPanel id="tools" className="navi-tabpanel">
            <div className={styles.placeholder}>Tool Calls panel</div>
          </TabPanel>
          <TabPanel id="errors" className="navi-tabpanel">
            <div className={styles.placeholder}>Errors panel</div>
          </TabPanel>
        </Tabs>
      );
    }

    // Default inspector
    return (
      <Tabs className="navi-tabs">
        <TabList className="navi-tablist">
          <Tab id="info" className="navi-tab">Info</Tab>
        </TabList>
        <TabPanel id="info" className="navi-tabpanel">
          <div className={styles.placeholder}>Select an item to view details</div>
        </TabPanel>
      </Tabs>
    );
  };

  return (
    <div className={styles.inspector}>
      <div className={styles.header}>
        <div className={styles.title}>Inspector</div>
        <Button className="navi-button-icon" onPress={inspector.close}>
          <X size={16} />
        </Button>
      </div>
      <div className={styles.content}>
        {renderTabs()}
      </div>
    </div>
  );
}
