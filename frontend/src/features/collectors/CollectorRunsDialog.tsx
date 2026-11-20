import {
  Chip,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Collector, RunState } from '../../api/types';
import ErrorBanner from '../../components/ErrorBanner';
import Modal from '../../components/Modal';
import { useCollectorRuns } from '../../hooks/useCollectors';
import { formatDateTime } from '../../lib/format';
import { formatRunCounts } from './format';
import './CollectorRunsDialog.scss';

const STATE_COLORS: Record<RunState, 'info' | 'success' | 'error'> = {
  running: 'info',
  succeeded: 'success',
  failed: 'error',
};

/** The run history of `collector`, newest first; closed while it is null. */
export default function CollectorRunsDialog({
  collector,
  onClose,
}: {
  collector: Collector | null;
  onClose: () => void;
}) {
  const { t, i18n } = useTranslation();
  const { data: runs, error, loading } = useCollectorRuns(collector?.id ?? null);
  const date = (iso: string) => formatDateTime(iso, i18n.language);

  return (
    <Modal
      open={collector !== null}
      title={t('collectors.runsTitle', { name: collector?.name ?? '' })}
      onClose={onClose}
    >
      <ErrorBanner message={error} />
      <TableContainer>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>{t('collectors.started')}</TableCell>
              <TableCell>{t('collectors.finished')}</TableCell>
              <TableCell>{t('collectors.mode')}</TableCell>
              <TableCell>{t('collectors.state')}</TableCell>
              <TableCell>{t('collectors.written')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {runs.map((run) => (
              <TableRow key={run.id}>
                <TableCell className="collector-runs__date">{date(run.startedAt)}</TableCell>
                <TableCell className="collector-runs__date">
                  {run.finishedAt ? date(run.finishedAt) : '–'}
                </TableCell>
                <TableCell>{t(`collectors.modes.${run.mode}`)}</TableCell>
                <TableCell>
                  <Chip
                    label={t(`collectors.states.${run.state}`)}
                    color={STATE_COLORS[run.state]}
                    size="small"
                    variant="outlined"
                  />
                  {run.error && (
                    <Typography variant="body2" color="error" className="collector-runs__error">
                      {run.error}
                    </Typography>
                  )}
                </TableCell>
                <TableCell>{formatRunCounts(run, t, i18n.language)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
      {!loading && runs.length === 0 && (
        <Typography variant="body2" color="text.secondary" className="collector-runs__empty">
          {t('collectors.noRuns')}
        </Typography>
      )}
    </Modal>
  );
}
