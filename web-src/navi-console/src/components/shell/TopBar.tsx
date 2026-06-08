import { ToggleButton } from 'react-aria-components';
import { PanelRightClose, PanelRightOpen } from 'lucide-react';
import { useInspector } from './AppShell';
import clsx from 'clsx';
import styles from './TopBar.module.css';

interface TopBarProps {
  version?: string;
}

export function TopBar({ version }: TopBarProps) {
  const inspector = useInspector();

  return (
    <header className={styles.topbar}>
      <div className={styles.left}>
      </div>

      <div className={styles.right}>
        {version && <span className={styles.version}>v{version}</span>}
        <ToggleButton
          className={clsx('navi-toggle', styles.inspectorToggle)}
          isSelected={inspector.isOpen}
          onChange={inspector.toggle}
          aria-label="Toggle inspector"
        >
          {inspector.isOpen ? <PanelRightClose size={18} /> : <PanelRightOpen size={18} />}
        </ToggleButton>
      </div>
    </header>
  );
}
