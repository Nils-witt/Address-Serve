import {
  Button,
  Chip,
  IconButton,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import HistoryIcon from '@mui/icons-material/History';
import KeyIcon from '@mui/icons-material/Key';
import { useTranslation } from 'react-i18next';
import type { Collector, CollectorStatus } from '../../api/types';
import { formatInterval, formatRunCounts } from '../../features/collectors/format';
import { formatDateTime } from '../../lib/format';
import './CollectorsTable.scss';

const STATUS_COLORS: Record<CollectorStatus, 'default' | 'info' | 'success' | 'warning' | 'error'> =
  {
    never: 'default',
    running: 'info',
    ok: 'success',
    stale: 'warning',
    failed: 'error',
  };

export default function CollectorsTable({
  collectors,
  loading,
  onCreate,
  onEdit,
  onShowRuns,
  onRotateToken,
  onDelete,
}: {
  collectors: Collector[];
  loading: boolean;
  onCreate: () => void;
  onEdit: (collector: Collector) => void;
  onShowRuns: (collector: Collector) => void;
  onRotateToken: (collector: Collector) => void;
  onDelete: (collector: Collector) => void;
}) {
  const { t, i18n } = useTranslation();
  const date = (iso: string) => formatDateTime(iso, i18n.language);

  return (
    <Paper className="collectors-table">
      <div className="collectors-table__header">
        <div>
          <Typography variant="h6" component="h1">
            {t('collectors.title')}
          </Typography>
          <Typography variant="body2" color="text.secondary">
            {t('collectors.intro')}
          </Typography>
        </div>
        <Button variant="contained" startIcon={<AddIcon />} onClick={onCreate}>
          {t('collectors.newCollector')}
        </Button>
      </div>
      <TableContainer>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>{t('collectors.name')}</TableCell>
              <TableCell>{t('collectors.status')}</TableCell>
              <TableCell>{t('collectors.lastRun')}</TableCell>
              <TableCell>{t('collectors.lastSeen')}</TableCell>
              <TableCell>{t('collectors.expectedInterval')}</TableCell>
              <TableCell align="right">{t('common.actions')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {collectors.map((collector) => {
              const run = collector.lastRun;
              return (
                <TableRow key={collector.id} hover>
                  <TableCell>
                    <Typography variant="body2">{collector.name}</Typography>
                    {collector.description && (
                      <Typography variant="caption" color="text.secondary">
                        {collector.description}
                      </Typography>
                    )}
                  </TableCell>
                  <TableCell>
                    <Tooltip title={t(`collectors.statusHelp.${collector.status}`)}>
                      <Chip
                        label={t(`collectors.statuses.${collector.status}`)}
                        color={STATUS_COLORS[collector.status]}
                        size="small"
                      />
                    </Tooltip>
                  </TableCell>
                  <TableCell>
                    {run ? (
                      <>
                        <Typography variant="body2" className="collectors-table__nowrap">
                          {date(run.startedAt)} · {t(`collectors.modes.${run.mode}`)}
                        </Typography>
                        <Typography variant="caption" color="text.secondary">
                          {formatRunCounts(run, t, i18n.language)}
                        </Typography>
                        {run.error && (
                          <Typography
                            variant="caption"
                            color="error"
                            className="collectors-table__error"
                          >
                            {run.error}
                          </Typography>
                        )}
                      </>
                    ) : (
                      t('collectors.noRuns')
                    )}
                  </TableCell>
                  <TableCell className="collectors-table__nowrap">
                    {collector.lastSeenAt ? date(collector.lastSeenAt) : t('collectors.neverSeen')}
                  </TableCell>
                  <TableCell className="collectors-table__nowrap">
                    {collector.expectedIntervalMinutes
                      ? formatInterval(collector.expectedIntervalMinutes, t)
                      : t('collectors.noInterval')}
                  </TableCell>
                  <TableCell align="right" className="collectors-table__nowrap">
                    <Tooltip title={t('collectors.runs')}>
                      <IconButton size="small" onClick={() => onShowRuns(collector)}>
                        <HistoryIcon fontSize="small" />
                      </IconButton>
                    </Tooltip>
                    <Tooltip title={t('common.edit')}>
                      <IconButton size="small" onClick={() => onEdit(collector)}>
                        <EditIcon fontSize="small" />
                      </IconButton>
                    </Tooltip>
                    <Tooltip title={t('collectors.rotateToken')}>
                      <IconButton size="small" onClick={() => onRotateToken(collector)}>
                        <KeyIcon fontSize="small" />
                      </IconButton>
                    </Tooltip>
                    <Tooltip title={t('common.delete')}>
                      <IconButton size="small" color="error" onClick={() => onDelete(collector)}>
                        <DeleteIcon fontSize="small" />
                      </IconButton>
                    </Tooltip>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </TableContainer>
      {!loading && collectors.length === 0 && (
        <Typography variant="body2" color="text.secondary" className="collectors-table__empty">
          {t('collectors.empty')}
        </Typography>
      )}
    </Paper>
  );
}
