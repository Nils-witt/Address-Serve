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
import DeleteIcon from '@mui/icons-material/Delete';
import HistoryIcon from '@mui/icons-material/History';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import RefreshIcon from '@mui/icons-material/Refresh';
import SettingsIcon from '@mui/icons-material/Settings';
import StopIcon from '@mui/icons-material/Stop';
import { useTranslation } from 'react-i18next';
import type { Collector } from '../../api/types';
import {
  collectorStatusColor,
  runStatusColor,
  totals,
} from '../../features/collectors/collectorFormat';
import { formatDateTime } from '../../lib/format';
import { localized } from '../../lib/localized';
import './CollectorsTable.scss';

function LastRun({ collector }: { collector: Collector }) {
  const { t, i18n } = useTranslation();
  const run = collector.lastRun;
  if (!run) return <>{t('collectors.neverRun')}</>;

  const sum = run.stats ? totals(run.stats) : null;
  return (
    <div className="collectors-table__last-run">
      <Chip
        size="small"
        color={runStatusColor[run.status]}
        label={t(`collectors.runStatus.${run.status}`)}
      />
      <Typography variant="caption" color="text.secondary">
        {formatDateTime(run.finishedAt ?? run.startedAt, i18n.language)}
        {sum && ` · ${t('collectors.summary', sum)}`}
      </Typography>
    </div>
  );
}

export default function CollectorsTable({
  collectors,
  loading,
  reloading,
  onReload,
  onRun,
  onCancel,
  onSettings,
  onHistory,
  onDelete,
}: {
  collectors: Collector[];
  loading: boolean;
  reloading: boolean;
  onReload: () => void;
  onRun: (c: Collector) => void;
  onCancel: (c: Collector) => void;
  onSettings: (c: Collector) => void;
  onHistory: (c: Collector) => void;
  onDelete: (c: Collector) => void;
}) {
  const { t, i18n } = useTranslation();

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
        <Button
          variant="outlined"
          startIcon={<RefreshIcon />}
          onClick={onReload}
          disabled={reloading}
        >
          {t('collectors.reload')}
        </Button>
      </div>
      <TableContainer>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>{t('collectors.collector')}</TableCell>
              <TableCell>{t('collectors.status')}</TableCell>
              <TableCell>{t('collectors.version')}</TableCell>
              <TableCell>{t('collectors.lastRun')}</TableCell>
              <TableCell align="right">{t('common.actions')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {collectors.map((c) => {
              const running = c.lastRun?.status === 'running';
              return (
                <TableRow key={c.id} hover>
                  <TableCell>
                    <Typography variant="body2">{c.name || c.id}</Typography>
                    <Typography variant="caption" color="text.secondary" component="div">
                      {c.id}
                      {c.description && ` · ${localized(c.description, i18n.language)}`}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Tooltip title={c.error ?? ''}>
                      <Chip
                        size="small"
                        variant="outlined"
                        color={collectorStatusColor[c.status]}
                        label={t(`collectors.collectorStatus.${c.status}`)}
                      />
                    </Tooltip>
                  </TableCell>
                  <TableCell>{c.version}</TableCell>
                  <TableCell>
                    <LastRun collector={c} />
                  </TableCell>
                  <TableCell align="right" className="collectors-table__actions">
                    {running ? (
                      <Tooltip title={t('collectors.cancel')}>
                        <IconButton size="small" color="warning" onClick={() => onCancel(c)}>
                          <StopIcon fontSize="small" />
                        </IconButton>
                      </Tooltip>
                    ) : (
                      <Tooltip title={t('collectors.run')}>
                        <span>
                          <IconButton
                            size="small"
                            color="primary"
                            disabled={c.status !== 'ok'}
                            onClick={() => onRun(c)}
                          >
                            <PlayArrowIcon fontSize="small" />
                          </IconButton>
                        </span>
                      </Tooltip>
                    )}
                    <Tooltip title={t('collectors.settings')}>
                      <span>
                        <IconButton
                          size="small"
                          disabled={c.settingsSchema.length === 0}
                          onClick={() => onSettings(c)}
                        >
                          <SettingsIcon fontSize="small" />
                        </IconButton>
                      </span>
                    </Tooltip>
                    <Tooltip title={t('collectors.history')}>
                      <IconButton size="small" onClick={() => onHistory(c)}>
                        <HistoryIcon fontSize="small" />
                      </IconButton>
                    </Tooltip>
                    {c.status !== 'ok' && (
                      <Tooltip title={t('common.delete')}>
                        <span>
                          <IconButton
                            size="small"
                            color="error"
                            disabled={running}
                            onClick={() => onDelete(c)}
                          >
                            <DeleteIcon fontSize="small" />
                          </IconButton>
                        </span>
                      </Tooltip>
                    )}
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
