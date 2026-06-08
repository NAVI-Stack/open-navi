import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from 'react';

interface RouteMatch {
  path: string;       // matched route pattern e.g. '/chats/:chatId'
  route: string;      // first segment e.g. 'chats'
  params: Record<string, string>;  // e.g. { chatId: '123' }
  query: URLSearchParams;
}

interface RouterContextValue {
  match: RouteMatch;
  navigate: (path: string, replace?: boolean) => void;
}

const RouterContext = createContext<RouterContextValue | null>(null);

export function useRoute(): RouteMatch {
  const ctx = useContext(RouterContext);
  if (!ctx) throw new Error('useRoute must be used within RouterProvider');
  return ctx.match;
}

export function useNavigate() {
  const ctx = useContext(RouterContext);
  if (!ctx) throw new Error('useNavigate must be used within RouterProvider');
  return ctx.navigate;
}

function parsePath(url: string): RouteMatch {
  // Handle hash routes for backward compat: #/overview -> /chats
  let pathname = url;
  if (url.startsWith('#')) {
    pathname = url.replace(/^#\/?/, '/');
  }
  
  const [pathPart, queryString] = pathname.split('?');
  const segments = pathPart.replace(/^\//, '').split('/').filter(Boolean);
  const route = segments[0] || 'chats';
  
  // Route pattern matching
  const params: Record<string, string> = {};
  
  // /chats/:chatId
  if (route === 'chats' && segments[1]) {
    params.chatId = segments[1];
  }
  // /projects/:projectId and /projects/:projectId/chats/:chatId  
  if (route === 'projects' && segments[1]) {
    params.projectId = segments[1];
    if (segments[2] === 'chats' && segments[3]) {
      params.chatId = segments[3];
    }
  }
  // /runs/:runId
  if (route === 'runs' && segments[1]) {
    params.runId = segments[1];
  }
  // /coder/:section
  if (route === 'coder' && segments[1]) {
    params.section = segments[1];
  }
  // /plugins/:section
  if (route === 'plugins' && segments[1]) {
    params.section = segments[1];
  }
  // /dev/:section
  if (route === 'dev' && segments[1]) {
    params.section = segments[1];
  }
  
  return {
    path: pathPart || '/chats',
    route,
    params,
    query: new URLSearchParams(queryString || ''),
  };
}

// Map old hash routes to new paths
function migrateHashRoute(hash: string): string | null {
  const old = hash.replace(/^#\/?/, '');
  const map: Record<string, string> = {
    'overview': '/overview',
    'chat': '/chats',
    'chats': '/chats',
    'projects': '/projects',
    'capabilities': '/plugins/skills',
    'config': '/settings',
    'appearance': '/settings',
    'logs': '/debug',
    'debug': '/debug',
    'proposals': '/proposals',
    'usage': '/usage',
    'scheduler': '/scheduler',
    'docs': '/docs',
    'workspaces': '/workspaces',
    'artifacts': '/artifacts',
  };
  return map[old] ?? null;
}

export function RouterProvider({ children }: { children: ReactNode }) {
  const [match, setMatch] = useState<RouteMatch>(() => {
    // Check for hash routes first and migrate
    if (window.location.hash) {
      const migrated = migrateHashRoute(window.location.hash);
      if (migrated) {
        window.history.replaceState(null, '', migrated);
        return parsePath(migrated);
      }
    }
    return parsePath(window.location.pathname + window.location.search);
  });

  const navigate = useCallback((path: string, replace = false) => {
    if (replace) {
      window.history.replaceState(null, '', path);
    } else {
      window.history.pushState(null, '', path);
    }
    setMatch(parsePath(path));
  }, []);

  useEffect(() => {
    const onPopState = () => {
      setMatch(parsePath(window.location.pathname + window.location.search));
    };
    window.addEventListener('popstate', onPopState);
    
    // Also handle hash changes for legacy compat
    const onHashChange = () => {
      if (window.location.hash) {
        const migrated = migrateHashRoute(window.location.hash);
        if (migrated) {
          window.history.replaceState(null, '', migrated);
          setMatch(parsePath(migrated));
        }
      }
    };
    window.addEventListener('hashchange', onHashChange);
    
    return () => {
      window.removeEventListener('popstate', onPopState);
      window.removeEventListener('hashchange', onHashChange);
    };
  }, []);

  return (
    <RouterContext.Provider value={{ match, navigate }}>
      {children}
    </RouterContext.Provider>
  );
}
