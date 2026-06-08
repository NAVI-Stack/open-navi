import { useEffect, useMemo, useRef, useState, type ComponentType } from 'react';
import {
  ArrowLeft,
  Check,
  ChevronRight,
  HeartHandshake,
  Loader2,
  RotateCcw,
  ShieldCheck,
  SkipForward,
  Sparkles,
  UserRound,
  Waves,
} from 'lucide-react';
import { useNavigate, useRoute } from '@/app/router';
import {
  type CeremonyPresenceMode,
  type CeremonyTrustBoundaries,
  useCeremony,
  useCompleteCeremony,
  useSkipCeremony,
  useStartCeremony,
} from '@/api/ceremony';
import styles from './CeremonyPage.module.css';

type CeremonyStep =
  | 'transition'
  | 'owner_recognition'
  | 'navi_presence'
  | 'trust_boundaries'
  | 'personalization_seed'
  | 'pact_summary';

type PresenceOption = {
  mode: CeremonyPresenceMode;
  title: string;
  body: string;
  Icon: ComponentType<{ size?: number }>;
};

type BoundaryKey = Exclude<keyof CeremonyTrustBoundaries, 'created_from' | 'owner_set'>;

type BoundaryOption = {
  key: BoundaryKey;
  label: string;
  body: string;
};

const STEP_ORDER: CeremonyStep[] = [
  'transition',
  'owner_recognition',
  'navi_presence',
  'trust_boundaries',
  'personalization_seed',
  'pact_summary',
];

const VISIBLE_STEPS: Array<{ id: CeremonyStep; label: string }> = [
  { id: 'owner_recognition', label: 'Name' },
  { id: 'navi_presence', label: 'Presence' },
  { id: 'trust_boundaries', label: 'Trust' },
  { id: 'personalization_seed', label: 'Remember' },
  { id: 'pact_summary', label: 'Pact' },
];

const PRESENCE_OPTIONS: PresenceOption[] = [
  {
    mode: 'calm_quiet',
    title: 'Calm and quiet',
    body: 'Less noise, less urgency, steady replies.',
    Icon: Waves,
  },
  {
    mode: 'warm_conversational',
    title: 'Warm and conversational',
    body: 'More relational language and room to talk things through.',
    Icon: HeartHandshake,
  },
  {
    mode: 'direct_strategic',
    title: 'Direct and strategic',
    body: 'Clear recommendations, useful challenge, planning bias.',
    Icon: ShieldCheck,
  },
  {
    mode: 'fast_focused',
    title: 'Fast and focused',
    body: 'Short answers, action first, minimal ceremony.',
    Icon: Sparkles,
  },
  {
    mode: 'balanced',
    title: 'Balanced',
    body: 'A mixed default that can adapt as you correct it.',
    Icon: UserRound,
  },
];

const BOUNDARY_OPTIONS: BoundaryOption[] = [
  {
    key: 'confirm_before_sending_messages',
    label: 'Sending messages',
    body: 'Ask before reaching out from your channels.',
  },
  {
    key: 'confirm_before_changing_files',
    label: 'Changing files',
    body: 'Ask before editing or creating files.',
  },
  {
    key: 'confirm_before_purchases',
    label: 'Making purchases or subscriptions',
    body: 'Ask before spending money or starting subscriptions.',
  },
  {
    key: 'confirm_before_remembering_sensitive_details',
    label: 'Remembering sensitive personal details',
    body: 'Ask before treating sensitive details as lasting context.',
  },
  {
    key: 'confirm_before_acting_on_inferred_preferences',
    label: 'Acting on inferred preferences',
    body: 'Ask before turning a guess into behavior.',
  },
  {
    key: 'confirm_before_interrupting_proactively',
    label: 'Interrupting proactively',
    body: 'Ask before becoming more interruptive.',
  },
  {
    key: 'confirm_before_external_changes',
    label: 'Making external changes',
    body: 'Ask before changing anything outside the console.',
  },
];

const DEFAULT_TRUST_BOUNDARIES: CeremonyTrustBoundaries = {
  confirm_before_sending_messages: true,
  confirm_before_changing_files: true,
  confirm_before_purchases: true,
  confirm_before_remembering_sensitive_details: true,
  confirm_before_acting_on_inferred_preferences: true,
  confirm_before_interrupting_proactively: false,
  confirm_before_external_changes: true,
};

export function CeremonyPage() {
  const route = useRoute();
  const navigate = useNavigate();
  const ceremony = useCeremony();
  const startCeremony = useStartCeremony();
  const completeCeremony = useCompleteCeremony();
  const skipCeremony = useSkipCeremony();
  const hydrated = useRef(false);
  const adjustMode = route.query.get('mode') === 'adjust';

  // First-run now goes through the chat interface. Only adjust mode uses this page.
  if (!adjustMode) {
    navigate('/chats', true);
    return null;
  }

  const [step, setStep] = useState<CeremonyStep>('transition');
  const [displayName, setDisplayName] = useState('');
  const [presenceMode, setPresenceMode] = useState<CeremonyPresenceMode>('balanced');
  const [boundaries, setBoundaries] = useState<CeremonyTrustBoundaries>(() => ({ ...DEFAULT_TRUST_BOUNDARIES }));
  const [personalizationSeed, setPersonalizationSeed] = useState('');

  useEffect(() => {
    const status = ceremony.data?.journeyState.status;
    if ((status === 'completed' || status === 'skipped') && !adjustMode) {
      navigate('/', true);
    }
  }, [adjustMode, ceremony.data?.journeyState.status, navigate]);

  useEffect(() => {
    if (!ceremony.data || hydrated.current) return;
    hydrated.current = true;
    setDisplayName(ceremony.data.ownerProfileSeed.displayName || '');
    setPresenceMode(ceremony.data.presencePreference.mode || 'balanced');
    setBoundaries(normalizeTrustBoundaries(ceremony.data.trustBoundaryDefaults));
    setPersonalizationSeed(ceremony.data.personalizationSeed.text || '');
    if (adjustMode) {
      setStep('owner_recognition');
    } else if (ceremony.data.journeyState.currentStep && isCeremonyStep(ceremony.data.journeyState.currentStep)) {
      setStep(ceremony.data.journeyState.currentStep);
    }
  }, [adjustMode, ceremony.data]);

  const currentIndex = STEP_ORDER.indexOf(step);
  const visibleStepIndex = Math.max(0, VISIBLE_STEPS.findIndex((item) => item.id === step));
  const actionPending = startCeremony.isPending || completeCeremony.isPending || skipCeremony.isPending;
  const actionError = startCeremony.error || completeCeremony.error || skipCeremony.error;
  const pactSummary = useMemo(
    () => buildPactSummary(displayName, presenceMode, boundaries, personalizationSeed),
    [boundaries, displayName, personalizationSeed, presenceMode],
  );

  const moveToStep = (next: CeremonyStep) => {
    setStep(next);
    if (next !== 'transition') {
      startCeremony.mutate(next);
    }
  };

  const goBack = () => {
    if (currentIndex <= 1) {
      moveToStep('transition');
      return;
    }
    moveToStep(STEP_ORDER[currentIndex - 1]);
  };

  const skip = async () => {
    const response = await skipCeremony.mutateAsync();
    navigate(response.redirect || '/', true);
  };

  const finish = async () => {
    const name = displayName.trim();
    if (!name) {
      moveToStep('owner_recognition');
      return;
    }
    const response = await completeCeremony.mutateAsync({
      displayName: name,
      presenceMode,
      trustBoundaries: boundaries,
      personalizationSeed: personalizationSeed.trim(),
    });
    navigate(response.redirect || '/', true);
  };

  const toggleBoundary = (key: BoundaryKey) => {
    setBoundaries((current) => ({
      ...current,
      [key]: !current[key],
    }));
  };

  if (ceremony.isLoading) {
    return (
      <div className={styles.page}>
        <div className={styles.centerState}>
          <Loader2 size={20} className={styles.spin} />
          <span>Opening NAVI Ceremony</span>
        </div>
      </div>
    );
  }

  if (ceremony.error) {
    return (
      <div className={styles.page}>
        <div className={styles.centerState}>
          <ShieldCheck size={20} />
          <span>Relationship choices are unavailable right now.</span>
          <button type="button" className={styles.secondaryButton} onClick={() => ceremony.refetch()}>
            <RotateCcw size={15} />
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.page}>
      <div className={styles.shell}>
        <aside className={styles.rail} aria-label="Ceremony progress">
          <div className={styles.mark}>
            <HeartHandshake size={18} />
          </div>
          <div>
            <h1>{adjustMode ? 'NAVI relationship' : 'NAVI Ceremony'}</h1>
            <p>{adjustMode ? 'Tune the choices NAVI uses with you.' : 'Make NAVI yours before the first real run.'}</p>
          </div>
          <ol className={styles.steps}>
            {VISIBLE_STEPS.map((item, index) => (
              <li
                key={item.id}
                className={index <= visibleStepIndex && step !== 'transition' ? styles.stepActive : undefined}
                aria-current={item.id === step ? 'step' : undefined}
              >
                <span>{index + 1}</span>
                {item.label}
              </li>
            ))}
          </ol>
        </aside>

        <main className={styles.stage}>
          {step === 'transition' && (
            <section className={styles.panel}>
              <div className={styles.promptIcon}>
                <Sparkles size={20} />
              </div>
              <h2>You're set up. Now let's make NAVI yours.</h2>
              <p className={styles.lede}>
                A few choices here shape how NAVI addresses you, shows up, asks before acting, and starts learning from what you give it.
              </p>
              <div className={styles.actions}>
                <button type="button" className={styles.primaryButton} onClick={() => moveToStep('owner_recognition')} disabled={actionPending}>
                  Begin Ceremony
                  <ChevronRight size={16} />
                </button>
                <button type="button" className={styles.secondaryButton} onClick={skip} disabled={actionPending}>
                  <SkipForward size={15} />
                  Skip for now
                </button>
              </div>
            </section>
          )}

          {step === 'owner_recognition' && (
            <section className={styles.panel}>
              <StepHeader icon={UserRound} title="What should I call you?" />
              <label className={styles.field}>
                <span>Preferred display name</span>
                <input
                  type="text"
                  value={displayName}
                  onChange={(event) => setDisplayName(event.target.value)}
                  placeholder="Your name"
                  autoComplete="name"
                  autoFocus
                />
              </label>
              <p className={styles.note}>NAVI learns this from you here. It is not copied from setup metadata.</p>
              <FlowActions
                canContinue={displayName.trim().length > 0}
                pending={actionPending}
                onBack={goBack}
                onNext={() => moveToStep('navi_presence')}
              />
            </section>
          )}

          {step === 'navi_presence' && (
            <section className={styles.panel}>
              <StepHeader icon={Waves} title="How should I show up for you?" />
              <div className={styles.optionGrid}>
                {PRESENCE_OPTIONS.map(({ mode, title, body, Icon }) => (
                  <button
                    type="button"
                    key={mode}
                    className={presenceMode === mode ? styles.optionActive : styles.option}
                    onClick={() => setPresenceMode(mode)}
                    aria-pressed={presenceMode === mode}
                  >
                    <Icon size={18} />
                    <span className={styles.optionTitle}>{title}</span>
                    <span className={styles.optionBody}>{body}</span>
                  </button>
                ))}
              </div>
              <FlowActions pending={actionPending} onBack={goBack} onNext={() => moveToStep('trust_boundaries')} />
            </section>
          )}

          {step === 'trust_boundaries' && (
            <section className={styles.panel}>
              <StepHeader icon={ShieldCheck} title="What should I ask before doing?" />
              <div className={styles.boundaryList}>
                {BOUNDARY_OPTIONS.map((item) => (
                  <label key={item.key} className={styles.boundaryItem}>
                    <input
                      type="checkbox"
                      checked={Boolean(boundaries[item.key])}
                      onChange={() => toggleBoundary(item.key)}
                    />
                    <span className={styles.checkboxVisual} aria-hidden="true">
                      {boundaries[item.key] ? <Check size={13} /> : null}
                    </span>
                    <span>
                      <span className={styles.optionTitle}>{item.label}</span>
                      <span className={styles.optionBody}>{item.body}</span>
                    </span>
                  </label>
                ))}
              </div>
              <FlowActions pending={actionPending} onBack={goBack} onNext={() => moveToStep('personalization_seed')} />
            </section>
          )}

          {step === 'personalization_seed' && (
            <section className={styles.panel}>
              <StepHeader icon={Sparkles} title="What's one thing you want me to remember from the beginning?" />
              <label className={styles.field}>
                <span>Optional</span>
                <textarea
                  value={personalizationSeed}
                  onChange={(event) => setPersonalizationSeed(event.target.value)}
                  placeholder="I prefer direct answers. Keep explanations short unless I ask. Help me stay focused."
                  rows={5}
                />
              </label>
              <p className={styles.note}>Sensitive or unclear details are staged for review instead of silently becoming memory.</p>
              <FlowActions pending={actionPending} onBack={goBack} onNext={() => moveToStep('pact_summary')} />
            </section>
          )}

          {step === 'pact_summary' && (
            <section className={styles.panel}>
              <StepHeader icon={HeartHandshake} title="Does this feel right?" />
              <div className={styles.pact}>
                {pactSummary.map((line) => (
                  <p key={line}>{line}</p>
                ))}
              </div>
              <div className={styles.actions}>
                <button type="button" className={styles.primaryButton} onClick={finish} disabled={actionPending || !displayName.trim()}>
                  Looks right
                  <Check size={16} />
                </button>
                <button type="button" className={styles.secondaryButton} onClick={() => moveToStep('owner_recognition')} disabled={actionPending}>
                  <RotateCcw size={15} />
                  Adjust
                </button>
                <button type="button" className={styles.ghostButton} onClick={skip} disabled={actionPending}>
                  <SkipForward size={15} />
                  Skip for now
                </button>
              </div>
            </section>
          )}

          {actionError && <div className={styles.errorText}>{actionError.message}</div>}
        </main>
      </div>
    </div>
  );
}

function StepHeader({ icon: Icon, title }: { icon: ComponentType<{ size?: number }>; title: string }) {
  return (
    <div className={styles.stepHeader}>
      <div className={styles.promptIcon}>
        <Icon size={20} />
      </div>
      <h2>{title}</h2>
    </div>
  );
}

function FlowActions({
  canContinue = true,
  pending,
  onBack,
  onNext,
}: {
  canContinue?: boolean;
  pending: boolean;
  onBack: () => void;
  onNext: () => void;
}) {
  return (
    <div className={styles.actions}>
      <button type="button" className={styles.secondaryButton} onClick={onBack} disabled={pending}>
        <ArrowLeft size={15} />
        Back
      </button>
      <button type="button" className={styles.primaryButton} onClick={onNext} disabled={pending || !canContinue}>
        Continue
        <ChevronRight size={16} />
      </button>
    </div>
  );
}

function isCeremonyStep(value: string): value is CeremonyStep {
  return STEP_ORDER.includes(value as CeremonyStep);
}

function normalizeTrustBoundaries(value: CeremonyTrustBoundaries | undefined): CeremonyTrustBoundaries {
  if (!value) return { ...DEFAULT_TRUST_BOUNDARIES };
  return {
    ...DEFAULT_TRUST_BOUNDARIES,
    ...value,
  };
}

export function buildPactSummary(
  displayName: string,
  presenceMode: CeremonyPresenceMode,
  boundaries: CeremonyTrustBoundaries,
  seed: string,
): string[] {
  const name = displayName.trim() || 'you';
  const askBefore = boundarySummary(boundaries) || 'the choices you mark as important';
  const remembered = seed.trim();
  const lines = [
    "Here's what I understand so far:",
    sentenceLine("I'll call you ", name),
    sentenceLine("I'll usually show up ", presenceLabel(presenceMode)),
    sentenceLine("I'll ask before ", askBefore),
  ];
  // Only surface a "remember" line when the owner actually provided a seed.
  if (remembered) {
    lines.push(sentenceLine("I'll remember ", remembered));
  }
  lines.push("I'll learn carefully, and you can correct what I know anytime.");
  return lines;
}

function presenceLabel(mode: CeremonyPresenceMode): string {
  return PRESENCE_OPTIONS.find((option) => option.mode === mode)?.title.toLowerCase() || 'balanced';
}

function boundarySummary(boundaries: CeremonyTrustBoundaries): string {
  const labels = BOUNDARY_OPTIONS
    .filter((option) => Boolean(boundaries[option.key]))
    .map((option) => option.label.toLowerCase());
  return labels.join(', ');
}

function sentenceLine(prefix: string, value: string): string {
  const trimmed = value.trim();
  if (!trimmed) return `${prefix.trim()}.`;
  return /[.!?]$/.test(trimmed) ? `${prefix}${trimmed}` : `${prefix}${trimmed}.`;
}
