type LabelRegistry = {
  set: (labels: string[]) => void;
  get: () => readonly string[];
  pick: () => string;
};

function createLabelRegistry(defaults: readonly string[]): LabelRegistry {
  let runtimeLabels: string[] = [...defaults];

  return {
    set(labels: string[]) {
      const next = labels.map((label) => label.trim()).filter(Boolean);
      runtimeLabels = next.length > 0 ? next : [...defaults];
    },
    get() {
      return runtimeLabels;
    },
    pick() {
      if (runtimeLabels.length === 0) {
        return defaults[0];
      }
      return runtimeLabels[Math.floor(Math.random() * runtimeLabels.length)];
    },
  };
}

const DEFAULT_PROJECT_NAME_LABELS = [
  'What are you working on?',
  'What is this project?',
  'What should we call it?',
  'Project name',
] as const;

const DEFAULT_PROJECT_DESCRIPTION_LABELS = [
  'Describe your project, goals, subject, etc...',
  'What are you trying to achieve?',
  'What are you working on?',
  "What's the goal?",
] as const;

const nameLabels = createLabelRegistry(DEFAULT_PROJECT_NAME_LABELS);
const descriptionLabels = createLabelRegistry(DEFAULT_PROJECT_DESCRIPTION_LABELS);

/** Replace project name field labels (e.g. from AI-generated copy). */
export function setProjectNameLabels(labels: string[]): void {
  nameLabels.set(labels);
}

export function getProjectNameLabels(): readonly string[] {
  return nameLabels.get();
}

export function pickProjectNameLabel(): string {
  return nameLabels.pick();
}

/** Replace description field labels (e.g. from AI-generated copy). */
export function setProjectDescriptionLabels(labels: string[]): void {
  descriptionLabels.set(labels);
}

export function getProjectDescriptionLabels(): readonly string[] {
  return descriptionLabels.get();
}

export function pickProjectDescriptionLabel(): string {
  return descriptionLabels.pick();
}
