import { type SubmitEvent, useState } from 'react';
import { Button, MenuItem, Stack, TextField } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Collector, CollectorInput, CreatedCollector } from '../../api/types';
import ErrorBanner from '../../components/ErrorBanner';
import { errorMessage } from '../../lib/errors';
import Modal from '../../components/Modal';
import TokenSecret from '../../components/TokenSecret';
import { formatInterval } from './format';
import './CollectorDialog.scss';

/** Expected interval choices in minutes; 0 means none. */
const INTERVAL_MINUTES = [0, 60, 6 * 60, 24 * 60, 7 * 24 * 60] as const;

function CollectorForm({
  collector,
  onClose,
  onSubmit,
}: {
  collector: Collector | null;
  onClose: () => void;
  onSubmit: (input: CollectorInput) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(collector?.name ?? '');
  const [description, setDescription] = useState(collector?.description ?? '');
  const [interval, setIntervalMinutes] = useState(collector?.expectedIntervalMinutes ?? 0);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  // Keep an interval set outside the UI selectable.
  const intervals: number[] = INTERVAL_MINUTES.includes(
    interval as (typeof INTERVAL_MINUTES)[number],
  )
    ? [...INTERVAL_MINUTES]
    : [...INTERVAL_MINUTES, interval].sort((a, b) => a - b);

  const handleSubmit = async (e: SubmitEvent<HTMLFormElement>) => {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      await onSubmit({
        name: name.trim(),
        description: description.trim(),
        expectedIntervalMinutes: interval || null,
      });
    } catch (err) {
      setError(errorMessage(err));
      setSubmitting(false);
    }
  };

  return (
    <Stack component="form" spacing={3} className="collector-dialog__body" onSubmit={handleSubmit}>
      <ErrorBanner message={error} />
      <TextField
        label={t('collectors.name')}
        helperText={t('collectors.nameHelp')}
        size="small"
        required
        autoFocus
        slotProps={{ htmlInput: { maxLength: 100 } }}
        value={name}
        onChange={(e) => setName(e.target.value)}
      />
      <TextField
        label={t('collectors.description')}
        size="small"
        multiline
        minRows={2}
        slotProps={{ htmlInput: { maxLength: 500 } }}
        value={description}
        onChange={(e) => setDescription(e.target.value)}
      />
      <TextField
        select
        label={t('collectors.expectedInterval')}
        helperText={t('collectors.expectedIntervalHelp')}
        size="small"
        value={interval}
        onChange={(e) => setIntervalMinutes(Number(e.target.value))}
      >
        {intervals.map((minutes) => (
          <MenuItem key={minutes} value={minutes}>
            {minutes ? formatInterval(minutes, t) : t('collectors.noInterval')}
          </MenuItem>
        ))}
      </TextField>
      <Stack direction="row" spacing={1} className="collector-dialog__actions">
        <Button onClick={onClose} disabled={submitting}>
          {t('common.cancel')}
        </Button>
        <Button type="submit" variant="contained" disabled={submitting}>
          {collector ? t('common.save') : t('common.create')}
        </Button>
      </Stack>
    </Stack>
  );
}

/** Registers a collector and then shows its token once, or edits one when
 * `collector` is set. Give it a new `key` for each opening, so it starts over
 * at the form and the last secret is dropped. */
export default function CollectorDialog({
  open,
  collector,
  onClose,
  onCreate,
  onUpdate,
}: {
  open: boolean;
  collector: Collector | null;
  onClose: () => void;
  onCreate: (input: CollectorInput) => Promise<CreatedCollector>;
  onUpdate: (id: string, input: CollectorInput) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [created, setCreated] = useState<CreatedCollector | null>(null);

  const title = created
    ? t('collectors.createdTitle')
    : collector
      ? t('collectors.editTitle')
      : t('collectors.newTitle');

  return (
    <Modal open={open} title={title} onClose={onClose}>
      {created ? (
        <TokenSecret label={created.name} secret={created.token} onClose={onClose} />
      ) : (
        <CollectorForm
          collector={collector}
          onClose={onClose}
          onSubmit={async (input) => {
            if (collector) {
              await onUpdate(collector.id, input);
              onClose();
            } else {
              setCreated(await onCreate(input));
            }
          }}
        />
      )}
    </Modal>
  );
}
