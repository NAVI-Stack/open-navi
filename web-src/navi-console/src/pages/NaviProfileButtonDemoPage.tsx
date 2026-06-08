import { useState } from 'react';
import { Button } from 'react-aria-components';
import {
  NAVI_PROFILE_STATUSES,
  NAVI_PROFILE_STATUS_DESCRIPTIONS,
  NAVI_PROFILE_STATUS_LABELS,
  NaviProfileButton,
  type NaviProfileStatus,
} from '@/components/shell/NaviProfileButton';
import clsx from 'clsx';
import styles from './NaviProfileButtonDemoPage.module.css';

const attentionStatuses: NaviProfileStatus[] = ['needs_attention', 'wants_attention'];
const activityStatuses: NaviProfileStatus[] = ['working', 'busy'];

export function NaviProfileButtonDemoPage() {
  const [status, setStatus] = useState<NaviProfileStatus>('active');

  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <div>
          <h1 className={styles.heading}>NAVI Profile Button</h1>
          <p className={styles.subheading}>Console-native single profile adaptation.</p>
        </div>
        <NaviProfileButton
          status={status}
          detail={NAVI_PROFILE_STATUS_DESCRIPTIONS[status]}
          size="demo"
          actionLabel="Preview NAVI profile"
        />
      </header>

      <section className={styles.section}>
        <div className={styles.sectionHeader}>
          <h2>Interactive State</h2>
          <span>{NAVI_PROFILE_STATUS_LABELS[status]}</span>
        </div>
        <div className={styles.previewRow}>
          <div className={styles.previewStage}>
            <NaviProfileButton
              status={status}
              detail={NAVI_PROFILE_STATUS_DESCRIPTIONS[status]}
              size="lg"
              actionLabel="Preview NAVI profile"
            />
          </div>
          <div className={styles.controls} aria-label="NAVI profile status">
            {NAVI_PROFILE_STATUSES.map((item) => (
              <Button
                key={item}
                className={clsx(styles.statusButton, status === item && styles.statusButtonActive)}
                onPress={() => setStatus(item)}
              >
                {NAVI_PROFILE_STATUS_LABELS[item]}
              </Button>
            ))}
          </div>
        </div>
      </section>

      <section className={styles.section}>
        <div className={styles.sectionHeader}>
          <h2>Status Variants</h2>
        </div>
        <div className={styles.statusGrid}>
          {NAVI_PROFILE_STATUSES.map((item) => (
            <article key={item} className={styles.statusCard}>
              <NaviProfileButton
                status={item}
                detail={NAVI_PROFILE_STATUS_DESCRIPTIONS[item]}
                size="demo"
                actionLabel={`Preview ${NAVI_PROFILE_STATUS_LABELS[item]}`}
              />
              <div>
                <h3>{NAVI_PROFILE_STATUS_LABELS[item]}</h3>
                <p>{NAVI_PROFILE_STATUS_DESCRIPTIONS[item]}</p>
              </div>
            </article>
          ))}
        </div>
      </section>

      <div className={styles.comparisonGrid}>
        <ComparisonSection title="Attention States" statuses={attentionStatuses} />
        <ComparisonSection title="Activity States" statuses={activityStatuses} />
      </div>
    </div>
  );
}

function ComparisonSection({
  title,
  statuses,
}: {
  title: string;
  statuses: NaviProfileStatus[];
}) {
  return (
    <section className={styles.section}>
      <div className={styles.sectionHeader}>
        <h2>{title}</h2>
      </div>
      <div className={styles.comparisonRow}>
        {statuses.map((status) => (
          <article key={status} className={styles.comparisonItem}>
            <NaviProfileButton
              status={status}
              detail={NAVI_PROFILE_STATUS_DESCRIPTIONS[status]}
              size="lg"
              actionLabel={`Preview ${NAVI_PROFILE_STATUS_LABELS[status]}`}
            />
            <div>
              <h3>{NAVI_PROFILE_STATUS_LABELS[status]}</h3>
              <p>{NAVI_PROFILE_STATUS_DESCRIPTIONS[status]}</p>
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
