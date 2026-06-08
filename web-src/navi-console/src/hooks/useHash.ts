import { useState, useEffect, useCallback } from 'react';

function getHash(): string {
  const raw = window.location.hash.replace(/^#\/?/, '');
  const [route] = raw.split('?');
  return route || 'overview';
}

export function useHash(): [string, (hash: string) => void] {
  const [hash, setHashState] = useState(getHash);

  useEffect(() => {
    const onHashChange = () => setHashState(getHash());
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  const setHash = useCallback((path: string) => {
    window.location.hash = `#/${path}`;
  }, []);

  return [hash, setHash];
}
