import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert } from '@mui/material';
import type { Collector, CollectorRun, CollectorSettingsInput } from '../api/types';
import ErrorBanner from '../components/ErrorBanner';
import CollectorDeleteDialog from '../features/collectors/CollectorDeleteDialog';
import CollectorRunsDialog from '../features/collectors/CollectorRunsDialog';
import CollectorSettingsDialog from '../features/collectors/CollectorSettingsDialog';
import { useApi } from '../hooks/useApi';
import { useCollectorEvents } from '../hooks/useCollectorEvents';
import { useCollectors } from '../hooks/useCollectors';
import { useConfirm } from '../hooks/useConfirm';
import { useInvalidate } from '../hooks/useInvalidate';
import { errorMessage } from '../lib/errors';
import CollectorsTable from './collectors-page/CollectorsTable';

type Open =
  | { dialog: 'settings'; collector: Collector }
  | { dialog: 'history'; collector: Collector; runId?: string }
  | { dialog: 'delete'; collector: Collector };

export default function CollectorsPage() {
  const { t } = useTranslation();
  const api = useApi();
  const confirm = useConfirm();
  const invalidate = useInvalidate();
  const { data: collectors, error: loadError, loading } = useCollectors();
  const live = useCollectorEvents();

  const [open, setOpen] = useState<Open | null>(null);
  // Bumped on each opening, so a dialog remounts with fresh state.
  const [dialogKey, setDialogKey] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [reloading, setReloading] = useState(false);

  const show = (next: Open) => {
    setDialogKey((k) => k + 1);
    setOpen(next);
  };
  const close = () => setOpen(null);

  // A run that ended changed streets, house numbers and geo objects, so
  // everything cached of those is refetched.
  const running = useRef(new Set<string>());
  useEffect(() => {
    const now = new Set(collectors.filter((c) => c.lastRun?.status === 'running').map((c) => c.id));
    const ended = [...running.current].some((id) => !now.has(id));
    running.current = now;
    if (ended) void invalidate('collectors');
  }, [collectors, invalidate]);

  const act = async (action: () => Promise<unknown>) => {
    setError(null);
    try {
      await action();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const onReload = () =>
    void act(async () => {
      setReloading(true);
      try {
        await api.reloadCollectors();
        await invalidate('collectors');
      } finally {
        setReloading(false);
      }
    });

  const onRun = (c: Collector) =>
    void act(async () => {
      const run = await api.startCollectorRun(c.id);
      await invalidate('collectors');
      show({ dialog: 'history', collector: c, runId: run.id });
    });

  const cancelRun = async (run: CollectorRun) => {
    const ok = await confirm({
      title: t('collectors.cancelTitle'),
      message: t('collectors.cancelMessage'),
      confirmLabel: t('collectors.cancel'),
      destructive: true,
    });
    if (!ok) return;
    await act(async () => {
      await api.cancelCollectorRun(run.id);
      await invalidate('collectors');
    });
  };

  const onCancel = (c: Collector) => {
    if (c.lastRun) void cancelRun(c.lastRun);
  };

  const onSaveSettings = async (id: string, input: CollectorSettingsInput) => {
    await api.updateCollectorSettings(id, input);
    await invalidate('collectors');
    close();
  };

  const onDelete = async (id: string, deleteData: boolean) => {
    await api.deleteCollector(id, deleteData);
    await invalidate('collectors');
    close();
  };

  return (
    <>
      <ErrorBanner message={error ?? loadError} />
      {live === 'reconnecting' && (
        <Alert severity="warning" className="error-banner">
          {t('collectors.liveUpdatesLost')}
        </Alert>
      )}
      <CollectorsTable
        collectors={collectors}
        loading={loading}
        reloading={reloading}
        onReload={onReload}
        onRun={onRun}
        onCancel={onCancel}
        onSettings={(c) => show({ dialog: 'settings', collector: c })}
        onHistory={(c) => show({ dialog: 'history', collector: c })}
        onDelete={(c) => show({ dialog: 'delete', collector: c })}
      />
      <CollectorSettingsDialog
        key={`settings-${dialogKey}`}
        collector={open?.dialog === 'settings' ? open.collector : null}
        onClose={close}
        onSave={onSaveSettings}
      />
      <CollectorRunsDialog
        key={`history-${dialogKey}`}
        collector={open?.dialog === 'history' ? open.collector : null}
        initialRunId={open?.dialog === 'history' ? open.runId : undefined}
        onClose={close}
        onCancel={(run) => void cancelRun(run)}
      />
      <CollectorDeleteDialog
        key={`delete-${dialogKey}`}
        collector={open?.dialog === 'delete' ? open.collector : null}
        onClose={close}
        onDelete={onDelete}
      />
    </>
  );
}
