import { useEffect, useState } from 'react';

interface TimeAgoProps {
  date?: string | null;
}

export function TimeAgo({ date }: TimeAgoProps) {
  const [display, setDisplay] = useState('');

  useEffect(() => {
    if (!date) {
      setDisplay('never');
      return;
    }

    const updateTime = () => {
      const d = new Date(date);
      if (isNaN(d.getTime())) {
        setDisplay('invalid date');
        return;
      }

      const now = new Date();
      const diffInSeconds = Math.floor((now.getTime() - d.getTime()) / 1000);

      if (diffInSeconds < 60) {
        setDisplay('just now');
      } else if (diffInSeconds < 3600) {
        const mins = Math.floor(diffInSeconds / 60);
        setDisplay(`${mins}m ago`);
      } else if (diffInSeconds < 86400) {
        const hours = Math.floor(diffInSeconds / 3600);
        setDisplay(`${hours}h ago`);
      } else if (diffInSeconds < 604800) {
        const days = Math.floor(diffInSeconds / 86400);
        setDisplay(`${days}d ago`);
      } else {
        setDisplay(d.toLocaleDateString());
      }
    };

    updateTime();
    const interval = setInterval(updateTime, 60000); // Update every minute
    return () => clearInterval(interval);
  }, [date]);

  if (!date) return <span className="time-ago">never</span>;

  return (
    <time className="time-ago" dateTime={date} title={new Date(date).toLocaleString()}>
      {display}
    </time>
  );
}
