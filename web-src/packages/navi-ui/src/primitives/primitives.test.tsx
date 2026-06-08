import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Button, Card, StatusBadge, Text, Stack, EmptyState } from '../index';

describe('@navi/ui primitives', () => {
  it('renders a Button with its label', () => {
    render(<Button>Click me</Button>);
    expect(screen.getByRole('button', { name: 'Click me' })).toBeInTheDocument();
  });

  it('renders a StatusBadge label', () => {
    render(<StatusBadge label="Running" variant="running" />);
    expect(screen.getByText('Running')).toBeInTheDocument();
  });

  it('renders Card children', () => {
    render(
      <Card>
        <span>card body</span>
      </Card>,
    );
    expect(screen.getByText('card body')).toBeInTheDocument();
  });

  it('renders Text and Stack content', () => {
    render(
      <Stack>
        <Text>typography</Text>
      </Stack>,
    );
    expect(screen.getByText('typography')).toBeInTheDocument();
  });

  it('renders an EmptyState title', () => {
    render(<EmptyState title="Nothing here" description="Try again later" />);
    expect(screen.getByText('Nothing here')).toBeInTheDocument();
  });
});
