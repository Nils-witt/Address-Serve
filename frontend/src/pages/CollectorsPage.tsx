import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Collector, CollectorInput } from '../api/types';
import ErrorBanner from '../components/ErrorBanner';
import Modal from '../components/Modal';
import TokenSecret from '../components/TokenSecret';
import CollectorDialog from '../features/collectors/CollectorDialog';
import CollectorRunsDialog from '../features/collectors/CollectorRunsDialog';
import { useApi } from '../hooks/useApi';
import { useCollectors } from '../hooks/useCollectors';
import { useConfirm } from '../hooks/useConfirm';
import { useInvalidate } from '../hooks/useInvalidate';
import { errorMessage } from '../lib/errors';
import CollectorsTable from './collectors-page/CollectorsTable';

export default function CollectorsPage() {
  const { t } = useTranslation();
  const api = useApi();
  const confirm = useConfirm();
  const invalidate = useInvalidate();
  const { data: collectors, error: loadError, loading } = useCollectors();

  const [dialogOpen, setDialogOpen] = useState(false);
  // Bumped on each opening, so the dialog remounts with a fresh form.
  const [dialogKey, setDialogKey] = useState(0);
  const [editing, setEditing] = useState<Collector | null>(null);
  const [runsOf, setRunsOf] = useState<Collector | null>(null);
  const [rotated, setRotated] = useState<{ name: string; token: string } | null>(null);
  const [error, setError] = useState<string | null>(null);

  const openDialog = (collector: Collector | null) => {
    setEditing(collector);
    setDialogKey((k) => k + 1);
    setDialogOpen(true);
  };

  const onCreate = async (input: CollectorInput) => {
    const created = await api.createCollector(input);
    await invalidate('collectors');
    return created;
  };

  const onUpdate = async (id: string, input: CollectorInput) => {
    await api.updateCollector(id, input);
    await invalidate('collectors');
  };

  const onRotateToken = async (collector: Collector) => {
    const ok = await confirm({
      title: t('collectors.rotateTitle'),
      message: t('collectors.rotateMessage', { name: collector.name }),
      confirmLabel: t('collectors.rotateToken'),
      destructive: true,
    });
    if (!ok) return;
    setError(null);
    try {
      setRotated({ name: collector.name, token: await api.rotateCollectorToken(collector.id) });
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const onDelete = async (collector: Collector) => {
    const ok = await confirm({
      title: t('collectors.deleteTitle'),
      message: t('collectors.deleteMessage', { name: collector.name }),
      confirmLabel: t('common.delete'),
      destructive: true,
    });
    if (!ok) return;
    setError(null);
    try {
      await api.deleteCollector(collector.id);
      await invalidate('collectors');
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  return (
    <>
      <ErrorBanner message={error ?? loadError} />
      <CollectorsTable
        collectors={collectors}
        loading={loading}
        onCreate={() => openDialog(null)}
        onEdit={openDialog}
        onShowRuns={setRunsOf}
        onRotateToken={(collector) => void onRotateToken(collector)}
        onDelete={(collector) => void onDelete(collector)}
      />
      <CollectorDialog
        key={dialogKey}
        open={dialogOpen}
        collector={editing}
        onClose={() => setDialogOpen(false)}
        onCreate={onCreate}
        onUpdate={onUpdate}
      />
      <CollectorRunsDialog collector={runsOf} onClose={() => setRunsOf(null)} />
      <Modal
        open={rotated !== null}
        title={t('collectors.rotatedTitle')}
        onClose={() => setRotated(null)}
      >
        {rotated && (
          <TokenSecret
            label={rotated.name}
            secret={rotated.token}
            onClose={() => setRotated(null)}
          />
        )}
      </Modal>
    </>
  );
}
