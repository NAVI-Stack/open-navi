/** Whether ceremony step controls should render on a chat message. */
export function shouldShowCeremonyControls(
  ceremonyActive: boolean,
  messageCeremonyStep: string | undefined,
  currentCeremonyStep: string | undefined,
): boolean {
  return (
    ceremonyActive &&
    !!messageCeremonyStep &&
    !!currentCeremonyStep &&
    messageCeremonyStep === currentCeremonyStep
  );
}
