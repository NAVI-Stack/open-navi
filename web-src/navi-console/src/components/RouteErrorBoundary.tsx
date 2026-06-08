import { Component, type ErrorInfo, type ReactNode } from 'react';
import { AlertTriangle } from 'lucide-react';
import styles from './RouteErrorBoundary.module.css';

type Props = {
  children: ReactNode;
  resetKey: string;
};

type State = {
  error: Error | null;
};

export class RouteErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('Console route render failed:', error, info);
  }

  componentDidUpdate(prevProps: Props) {
    if (prevProps.resetKey !== this.props.resetKey && this.state.error) {
      this.setState({ error: null });
    }
  }

  render() {
    if (!this.state.error) {
      return this.props.children;
    }

    return (
      <div className={styles.panel} role="alert">
        <AlertTriangle size={18} />
        <div>
          <h2 className={styles.title}>Page Render Failed</h2>
          <p className={styles.message}>
            This Console page hit unexpected data while rendering. Use the sidebar to switch pages, or refresh after the backend snapshot updates.
          </p>
          <pre className={styles.error}>{this.state.error.message}</pre>
          <button className={styles.retry} type="button" onClick={() => this.setState({ error: null })}>
            Retry render
          </button>
        </div>
      </div>
    );
  }
}
