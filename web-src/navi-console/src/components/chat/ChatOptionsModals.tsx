import React, { useState, useEffect } from 'react';
import { Dialog, Modal, ModalOverlay, Button, Form, Label } from 'react-aria-components';
import { useQueryClient } from '@tanstack/react-query';
import { updateChat } from '@/api/chats';
import { useProjects } from '@/api/projects';
import { X, Check, FolderKanban, FolderMinus } from 'lucide-react';
import { ICON_MAP } from '@/lib/iconMap';
import styles from './ChatOptionsModals.module.css';

interface RenameChatModalProps {
  chat: any | null;
  isOpen: boolean;
  onClose: () => void;
}

export function RenameChatModal({ chat, isOpen, onClose }: RenameChatModalProps) {
  const [title, setTitle] = useState('');
  const queryClient = useQueryClient();

  useEffect(() => {
    if (isOpen && chat) {
      setTitle(chat.title || '');
    }
  }, [isOpen, chat]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!chat) return;

    try {
      await updateChat(chat.chat_id, { title: title.trim() });
      queryClient.invalidateQueries({ queryKey: ['chats'] });
      onClose();
    } catch (err) {
      console.error('Failed to rename chat:', err);
    }
  };

  if (!isOpen || !chat) return null;

  return (
    <ModalOverlay isOpen={isOpen} className="navi-modal-overlay" isDismissable onOpenChange={(open) => !open && onClose()}>
      <Modal className="navi-modal" style={{ width: '400px' }}>
        <Dialog className="navi-dialog" aria-label="Rename Chat">
          {({ close }) => (
            <Form onSubmit={handleSubmit} className={styles.form}>
              <div className={styles.header}>
                <h2 className={styles.title}>Rename Chat</h2>
                <Button onPress={close} className="navi-button-icon" aria-label="Close">
                  <X size={16} />
                </Button>
              </div>

              <div className={styles.field}>
                <Label className={styles.label}>Chat Title</Label>
                <input
                  type="text"
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                  className="navi-input"
                  placeholder="Untitled Chat"
                  required
                  autoFocus
                />
              </div>

              <div className={styles.footer}>
                <Button onPress={close} className="navi-button navi-button-ghost">Cancel</Button>
                <Button type="submit" className="navi-button navi-button-primary">Save Changes</Button>
              </div>
            </Form>
          )}
        </Dialog>
      </Modal>
    </ModalOverlay>
  );
}

interface MoveProjectModalProps {
  chat: any | null;
  isOpen: boolean;
  onClose: () => void;
}

export function MoveProjectModal({ chat, isOpen, onClose }: MoveProjectModalProps) {
  const [projectId, setProjectId] = useState<string>('');
  const { data: projectsData } = useProjects();
  const projects = projectsData?.items || [];
  const queryClient = useQueryClient();

  useEffect(() => {
    if (isOpen && chat) {
      setProjectId(chat.project_id || '');
    }
  }, [isOpen, chat]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!chat) return;

    try {
      await updateChat(chat.chat_id, { project_id: projectId || null });
      queryClient.invalidateQueries({ queryKey: ['chats'] });
      queryClient.invalidateQueries({ queryKey: ['project-chats'] });
      onClose();
    } catch (err) {
      console.error('Failed to move chat:', err);
    }
  };

  if (!isOpen || !chat) return null;

  return (
    <ModalOverlay isOpen={isOpen} className="navi-modal-overlay" isDismissable onOpenChange={(open) => !open && onClose()}>
      <Modal className="navi-modal" style={{ width: '400px' }}>
        <Dialog className="navi-dialog" aria-label="Move to Project">
          {({ close }) => (
             <Form onSubmit={handleSubmit} className={styles.form}>
              <div className={styles.header}>
                <h2 className={styles.title}>Move to Project</h2>
                <Button onPress={close} className="navi-button-icon" aria-label="Close">
                  <X size={16} />
                </Button>
              </div>

              <div className={styles.field}>
                <Label className={styles.label}>Project</Label>
                <div className={styles.projectList}>
                  <div
                    className={`${styles.projectItem} ${projectId === '' ? styles.selected : ''}`}
                    onClick={() => setProjectId('')}
                  >
                    <div className={styles.projectItemLeft}>
                      <FolderMinus size={16} />
                      <span>None (Remove from project)</span>
                    </div>
                    {projectId === '' && <Check size={16} />}
                  </div>

                  {projects.map((p) => {
                    const IconComp = ICON_MAP[p.icon || 'folder'] || FolderKanban;
                    return (
                      <div
                        key={p.project_id}
                        className={`${styles.projectItem} ${projectId === p.project_id ? styles.selected : ''}`}
                        onClick={() => setProjectId(p.project_id)}
                      >
                        <div className={styles.projectItemLeft}>
                          <IconComp size={16} style={{ color: projectId === p.project_id ? undefined : p.color }} />
                          <span>{p.title}</span>
                        </div>
                        {projectId === p.project_id && <Check size={16} />}
                      </div>
                    );
                  })}
                </div>
              </div>

              <div className={styles.footer}>
                <Button onPress={close} className="navi-button navi-button-ghost">Cancel</Button>
                <Button type="submit" className="navi-button navi-button-primary">Move</Button>
              </div>
            </Form>
          )}
        </Dialog>
      </Modal>
    </ModalOverlay>
  );
}
