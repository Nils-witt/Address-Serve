import { useState } from 'react';
import {
  Button,
  Checkbox,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  FormControlLabel,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Collector } from '../../api/types';
import ErrorBanner from '../../components/ErrorBanner';
import { errorMessage } from '../../lib/errors';

/** Deletes a missing or invalid collector, optionally with the rows it owns. */
export default function CollectorDeleteDialog({
  collector,
  onClose,
  onDelete,
}: {
  collector: Collector | null;
  onClose: () => void;
  onDelete: (id: string, deleteData: boolean) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [deleteData, setDeleteData] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);
  if (!collector) return null;

  const handleDelete = async () => {
    setError(null);
    setDeleting(true);
    try {
      await onDelete(collector.id, deleteData);
    } catch (err) {
      setError(errorMessage(err));
      setDeleting(false);
    }
  };

  return (
    <Dialog open onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        {t('collectors.deleteTitle', { name: collector.name || collector.id })}
      </DialogTitle>
      <DialogContent>
        <ErrorBanner message={error} />
        <DialogContentText>{t('collectors.deleteMessage')}</DialogContentText>
        <FormControlLabel
          control={
            <Checkbox checked={deleteData} onChange={(e) => setDeleteData(e.target.checked)} />
          }
          label={t('collectors.deleteData')}
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={deleting}>
          {t('common.cancel')}
        </Button>
        <Button
          color="error"
          variant="contained"
          onClick={() => void handleDelete()}
          disabled={deleting}
        >
          {t('common.delete')}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
