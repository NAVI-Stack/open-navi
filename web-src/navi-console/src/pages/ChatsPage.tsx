import { useState } from 'react';
import { SearchField, Input, Checkbox, MenuTrigger, Popover, Menu, MenuItem, Button } from 'react-aria-components';
import { MessageSquare, Search, MoreVertical, Check, Archive, Trash2, CheckSquare, Minus, X, Edit, FolderUp, FolderMinus } from 'lucide-react';
import { useChats, archiveChat, deleteChat, updateChat } from '@/api/chats';
import { useNavigate, useRoute } from '@/app/router';
import { useQueryClient } from '@tanstack/react-query';
import { isDraftChatId, openNewChat } from '@/lib/newChat';
import { EmptyState } from '@/components/ui/EmptyState';
import { TimeAgo } from '@/components/ui/TimeAgo';
import { useToast } from '@/components/ui/Toast';
import { useConfirm } from '@/components/ui/ConfirmDialog';
import { RenameChatModal, MoveProjectModal } from '@/components/chat/ChatOptionsModals';
import styles from './ChatsPage.module.css';

export function ChatsPage() {
  const { data: chats, isLoading } = useChats();
  const { params } = useRoute();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const toast = useToast();
  const confirm = useConfirm();
  const [search, setSearch] = useState('');
  const [selectedChats, setSelectedChats] = useState<Set<string>>(new Set());
  const [editingChat, setEditingChat] = useState<any | null>(null);
  const [modalType, setModalType] = useState<'rename' | 'move' | null>(null);

  const handleNewChat = () => {
    openNewChat(navigate, { replace: isDraftChatId(params.chatId) });
  };

  const handleMenuAction = async (action: string | number, chatId: string) => {
    if (action === 'select') {
      const next = new Set(selectedChats);
      if (next.has(chatId)) next.delete(chatId);
      else next.add(chatId);
      setSelectedChats(next);
    } else if (action === 'archive') {
      const previousChats = queryClient.getQueryData<any[]>(['chats', undefined]);
      if (previousChats) {
        queryClient.setQueryData(['chats', undefined], previousChats.filter(c => c.chat_id !== chatId));
      }
      try {
        await archiveChat(chatId);
        queryClient.invalidateQueries({ queryKey: ['chats'] });
      } catch (e) {
        console.error('Failed to archive chat', e);
        if (previousChats) {
          queryClient.setQueryData(['chats', undefined], previousChats);
        }
        toast.error('Failed to archive chat');
      }
    } else if (action === 'delete') {
      const ok = await confirm({
        title: 'Delete chat?',
        description: 'This permanently removes the chat and its transcript. This cannot be undone.',
        confirmLabel: 'Delete',
        danger: true,
      });
      if (!ok) return;
      const previousChats = queryClient.getQueryData<any[]>(['chats', undefined]);
      if (previousChats) {
        queryClient.setQueryData(['chats', undefined], previousChats.filter(c => c.chat_id !== chatId));
      }
      try {
        await deleteChat(chatId);
        queryClient.invalidateQueries({ queryKey: ['chats'] });
      } catch (e) {
        console.error('Failed to delete chat', e);
        if (previousChats) {
          queryClient.setQueryData(['chats', undefined], previousChats);
        }
        toast.error('Failed to delete chat');
      }
    } else if (action === 'rename') {
      const chat = chats?.find(c => c.chat_id === chatId);
      if (chat) {
        setEditingChat(chat);
        setModalType('rename');
      }
    } else if (action === 'move') {
      const chat = chats?.find(c => c.chat_id === chatId);
      if (chat) {
        setEditingChat(chat);
        setModalType('move');
      }
    } else if (action === 'remove-project') {
      try {
        await updateChat(chatId, { project_id: null });
        queryClient.invalidateQueries({ queryKey: ['chats'] });
        queryClient.invalidateQueries({ queryKey: ['project-chats'] });
      } catch (e) {
        console.error('Failed to remove from project', e);
        toast.error('Failed to remove from project');
      }
    }
  };

  const handleBulkAction = async (action: 'archive' | 'delete') => {
    const idsToProcess = Array.from(selectedChats);
    if (action === 'delete') {
      const ok = await confirm({
        title: `Delete ${idsToProcess.length} chat${idsToProcess.length === 1 ? '' : 's'}?`,
        description: 'This permanently removes the selected chats and their transcripts. This cannot be undone.',
        confirmLabel: 'Delete',
        danger: true,
      });
      if (!ok) return;
    }
    const previousChats = queryClient.getQueryData<any[]>(['chats', undefined]);

    if (previousChats) {
      queryClient.setQueryData(['chats', undefined], previousChats.filter(c => !idsToProcess.includes(c.chat_id)));
    }

    try {
      if (action === 'archive') {
        await Promise.all(idsToProcess.map(id => archiveChat(id)));
      } else {
        await Promise.all(idsToProcess.map(id => deleteChat(id)));
      }
      setSelectedChats(new Set());
      queryClient.invalidateQueries({ queryKey: ['chats'] });
    } catch (e) {
      console.error(`Failed to perform bulk ${action} action`, e);
      if (previousChats) {
        queryClient.setQueryData(['chats', undefined], previousChats);
      }
      toast.error(`Failed to ${action} some chats`);
    }
  };

  const handleRowClick = (e: React.MouseEvent, chatId: string) => {
    const target = e.target as HTMLElement;
    if (target.closest('button') || target.closest('input')) {
      return; // Ignore clicks from interactive children
    }
    navigate(`/chats/${chatId}`);
  };

  if (isLoading) {
    return <div className={styles.loading}>Loading chats...</div>;
  }

  if (!chats || chats.length === 0) {
    return (
      <EmptyState
        icon={<MessageSquare size={24} />}
        title="No chats yet"
        description="Create a new chat to get started."
        action={{ label: 'New Chat', onPress: handleNewChat }}
      />
    );
  }

  const filteredChats = chats.filter(chat =>
    (chat.title || 'Untitled Chat').toLowerCase().includes(search.toLowerCase())
  );

  const isMultiSelectMode = selectedChats.size > 0;
  const isAllSelected = filteredChats.length > 0 && selectedChats.size === filteredChats.length;
  const isIndeterminate = selectedChats.size > 0 && selectedChats.size < filteredChats.length;

  return (
    <div className={styles.container}>
      {!isMultiSelectMode ? (
        <div className={styles.header}>
          <h1 className={styles.title}>Chats</h1>
          <div className={styles.actions}>
            <SearchField value={search} onChange={setSearch} className="navi-searchfield">
              <Search size={16} className={styles.searchIcon} />
              <Input placeholder="Search chats..." className="navi-searchfield-input" />
            </SearchField>
            <Button className="navi-button navi-button-primary" onPress={handleNewChat}>
              New chat
            </Button>
          </div>
        </div>
      ) : (
        <div className={styles.bulkActionBar}>
          <Checkbox
            className="navi-checkbox"
            isIndeterminate={isIndeterminate}
            isSelected={isAllSelected}
            onChange={(isSelected) => {
              if (isSelected) {
                setSelectedChats(new Set(filteredChats.map(c => c.chat_id)));
              } else {
                setSelectedChats(new Set());
              }
            }}
            aria-label="Select all"
          >
            {({ isSelected, isIndeterminate }) => (
              <div className="navi-checkbox-indicator">
                {isIndeterminate ? <Minus className="navi-checkbox-icon" /> : isSelected && <Check className="navi-checkbox-icon" />}
              </div>
            )}
          </Checkbox>
          <span className={styles.bulkActionText}>{selectedChats.size} selected</span>
          <div className={styles.bulkActions}>
            <Button aria-label="Archive selected" className="navi-button-icon" onPress={() => handleBulkAction('archive')}>
              <Archive size={16} />
            </Button>
            <Button aria-label="Delete selected" className="navi-button-icon" onPress={() => handleBulkAction('delete')}>
              <Trash2 size={16} />
            </Button>
          </div>
          <div style={{ flex: 1 }} />
          <Button aria-label="Clear selection" className="navi-button-icon" onPress={() => setSelectedChats(new Set())}>
            <X size={16} />
          </Button>
        </div>
      )}

      {filteredChats.length === 0 ? (
        <EmptyState
          icon={<MessageSquare size={24} />}
          title="No matches found"
          description={`We couldn't find any chats matching "${search}"`}
          action={{ label: 'Clear search', onPress: () => setSearch('') }}
        />
      ) : (
        <div className={styles.list} data-multiselect={isMultiSelectMode}>
          {filteredChats.map(chat => (
            <div 
              key={chat.chat_id} 
              className={styles.chatRow}
              data-selected={selectedChats.has(chat.chat_id) ? 'true' : undefined}
              onClick={(e) => handleRowClick(e, chat.chat_id)}
            >
              <Checkbox 
                className={`navi-checkbox ${styles.checkboxWrapper}`}
                isSelected={selectedChats.has(chat.chat_id)}
                onChange={(isSelected) => {
                  const next = new Set(selectedChats);
                  if (isSelected) next.add(chat.chat_id);
                  else next.delete(chat.chat_id);
                  setSelectedChats(next);
                }}
                onClick={(e) => e.stopPropagation()}
                aria-label={`Select chat ${chat.title || 'Untitled'}`}
              >
                {({ isSelected }) => (
                  <div className="navi-checkbox-indicator">
                    {isSelected && <Check className="navi-checkbox-icon" />}
                  </div>
                )}
              </Checkbox>

              <MessageSquare size={16} className={styles.chatIcon} />
              
              <div className={styles.chatInfo}>
                <h3 className={styles.chatTitle}>{chat.title || 'Untitled Chat'}</h3>
              </div>

              <div className={styles.chatRight}>
                <div className={styles.dateText}>
                  <TimeAgo date={chat.updated_at} />
                </div>

                <MenuTrigger>
                  <Button aria-label="Chat options" className={`navi-button-icon ${styles.kebabBtn}`}>
                    <MoreVertical size={16} />
                  </Button>
                  <Popover className="navi-popover" placement="bottom end">
                    <Menu 
                      onAction={(key) => handleMenuAction(key, chat.chat_id)}
                      style={{ outline: 'none' }}
                    >
                      <MenuItem className="navi-menu-item" id="select">
                        <CheckSquare size={14} /> Select
                      </MenuItem>
                      <MenuItem className="navi-menu-item" id="rename">
                        <Edit size={14} /> Rename
                      </MenuItem>
                      <MenuItem className="navi-menu-item" id="move">
                        <FolderUp size={14} /> Move to project
                      </MenuItem>
                      {chat.project_id && (
                        <MenuItem className="navi-menu-item" id="remove-project">
                          <FolderMinus size={14} /> Remove from project
                        </MenuItem>
                      )}
                      <MenuItem className="navi-menu-item" id="archive">
                        <Archive size={14} /> Archive
                      </MenuItem>
                      <MenuItem className="navi-menu-item navi-menu-item-danger" id="delete">
                        <Trash2 size={14} /> Delete
                      </MenuItem>
                    </Menu>
                  </Popover>
                </MenuTrigger>
              </div>
            </div>
          ))}
        </div>
      )}

      {modalType === 'rename' && (
        <RenameChatModal
          chat={editingChat}
          isOpen={true}
          onClose={() => { setModalType(null); setEditingChat(null); }}
        />
      )}
      {modalType === 'move' && (
        <MoveProjectModal
          chat={editingChat}
          isOpen={true}
          onClose={() => { setModalType(null); setEditingChat(null); }}
        />
      )}
    </div>
  );
}
