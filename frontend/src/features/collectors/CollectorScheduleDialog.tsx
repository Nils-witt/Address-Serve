import { type SubmitEvent, useState } from 'react';
import { Button, Chip, Stack, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Collector } from '../../api/types';
import ErrorBanner from '../../components/ErrorBanner';
import Modal from '../../components/Modal';
import { errorMessage } from '../../lib/errors';
import './CollectorSettingsDialog.scss';

/** Common schedules offered as one-click fills of the field. */
const EXAMPLES = ['0 3 * * *', 'CRON_TZ=Europe/Berlin 0 3 * * *', '@hourly', '@every 6h'];

/** Sets or removes the cron expression that starts a collector's runs. The
 * server validates the expression; its error shows in the dialog. Give it a
 * new `key` for each opening, so it starts from the stored value. */
export default function CollectorScheduleDialog({
  collector,
  onClose,
  onSave,
}: {
  collector: Collector | null;
  onClose: () => void;
  onSave: (id: string, schedule: string) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [schedule, setSchedule] = useState(collector?.schedule ?? '');
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  if (!collector) return null;

  const save = async (value: string) => {
    setError(null);
    setSaving(true);
    try {
      await onSave(collector.id, value.trim());
    } catch (err) {
      setError(errorMessage(err));
      setSaving(false);
    }
  };

  const handleSubmit = (e: SubmitEvent<HTMLFormElement>) => {
    e.preventDefault();
    void save(schedule);
  };

  return (
    <Modal
      open
      title={t('collectors.scheduleTitle', { name: collector.name || collector.id })}
      onClose={onClose}
    >
      <Stack
        component="form"
        spacing={3}
        className="collector-settings-dialog__body"
        onSubmit={handleSubmit}
      >
        <ErrorBanner message={error} />
        <TextField
          label={t('collectors.scheduleLabel')}
          size="small"
          placeholder="0 3 * * *"
          autoComplete="off"
          helperText={t('collectors.scheduleHelp')}
          slotProps={{ htmlInput: { spellCheck: false }, inputLabel: { shrink: true } }}
          value={schedule}
          onChange={(e) => setSchedule(e.target.value)}
        />
        <Stack spacing={1}>
          <Typography variant="caption" color="text.secondary">
            {t('collectors.scheduleExamples')}
          </Typography>
          <Stack direction="row" spacing={1} useFlexGap sx={{ flexWrap: 'wrap' }}>
            {EXAMPLES.map((ex) => (
              <Chip
                key={ex}
                size="small"
                variant="outlined"
                label={ex}
                onClick={() => setSchedule(ex)}
              />
            ))}
          </Stack>
        </Stack>
        <Stack direction="row" spacing={1} className="collector-settings-dialog__actions">
          {collector.schedule && (
            <Button color="error" onClick={() => void save('')} disabled={saving}>
              {t('collectors.scheduleRemove')}
            </Button>
          )}
          <Button onClick={onClose} disabled={saving}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" variant="contained" disabled={saving}>
            {t('common.save')}
          </Button>
        </Stack>
      </Stack>
    </Modal>
  );
}
