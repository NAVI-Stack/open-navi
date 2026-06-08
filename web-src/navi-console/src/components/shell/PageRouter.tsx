import { useRoute } from '@/app/router';
import { ChatPage } from '@/pages/ChatPage';
import { ChatsPage } from '@/pages/ChatsPage';
import { ProjectsPage } from '@/pages/ProjectsPage';
import { RunsPage } from '@/pages/RunsPage';
import { RunDetailPage } from '@/pages/RunDetailPage';
import { CoderPage } from '@/pages/CoderPage';
import { PluginsPage } from '@/pages/PluginsPage';
import { PluginDetailPage } from '@/pages/plugin-detail/PluginDetailPage';
import { SettingsPage } from '@/pages/SettingsPage';
import { CeremonyPage } from '@/pages/CeremonyPage';
import { DebugPage } from '@/pages/DebugPage';
import { NaviProfileButtonDemoPage } from '@/pages/NaviProfileButtonDemoPage';
import { GenUIDemoPage } from '@/pages/GenUIDemoPage';
import { Workspaces } from '@/pages/Workspaces';
import { Artifacts } from '@/pages/Artifacts';
import { Proposals } from '@/pages/Proposals';
import { Usage } from '@/pages/Usage';
import { Scheduler } from '@/pages/Scheduler';
import { Docs } from '@/pages/Docs';
import { Overview } from '@/pages/Overview';


export function PageRouter() {
  const { route, params } = useRoute();

  // /chats/:chatId -> ChatPage
  if (route === 'chats' && params.chatId) {
    return <ChatPage chatId={params.chatId} />;
  }
  
  // /projects/:projectId/chats/:chatId -> ChatPage with project context
  if (route === 'projects' && params.chatId) {
    return <ChatPage chatId={params.chatId} projectId={params.projectId} />;
  }

  switch (route) {
    case 'chats':
      return <ChatsPage />;
    case 'projects':
      return <ProjectsPage projectId={params.projectId} />;
    case 'runs':
      return params.runId ? <RunDetailPage runId={params.runId} /> : <RunsPage />;
    case 'coder':
      return <CoderPage section={params.section} />;
    case 'plugins':
      // Reserved sub-paths render the list (e.g. legacy /plugins/skills tab).
      if (params.section && params.section !== 'skills' && params.section !== 'plugins') {
        return <PluginDetailPage pluginId={params.section} />;
      }
      return <PluginsPage />;
    case 'settings':
      return <SettingsPage />;
    case 'ceremony':
      return <CeremonyPage />;
    case 'debug':
      return <DebugPage />;
    case 'overview':
      return <Overview />;
    case 'workspaces':
      return <Workspaces />;
    case 'artifacts':
      return <Artifacts />;
    case 'proposals':
      return <Proposals />;
    case 'usage':
      return <Usage />;
    case 'scheduler':
      return <Scheduler />;
    case 'docs':
      return <Docs />;
    case 'dev':
      if (params.section === 'navi-profile-button') {
        return <NaviProfileButtonDemoPage />;
      }
      if (params.section === 'genui') {
        return <GenUIDemoPage />;
      }
      return <ChatsPage />;
    default:
      return <ChatsPage />;
  }
}
