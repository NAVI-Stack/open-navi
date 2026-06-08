type MessageWithID = {
  id?: string;
};

export function shouldSyncServerMessages(
  serverMessages: readonly MessageWithID[],
  localMessages: readonly MessageWithID[],
  status: string,
): boolean {
  if (status !== 'ready') return false;
  if (serverMessages.length === 0) return false;
  if (localMessages.length === 0) return true;
  if (serverMessages.length > localMessages.length) return true;

  const serverTailID = serverMessages[serverMessages.length - 1]?.id;
  const localTailID = localMessages[localMessages.length - 1]?.id;
  return Boolean(serverTailID && serverTailID !== localTailID);
}
