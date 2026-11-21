import { useLayoutEffect, useRef, useState } from 'react';
import {
  Button,
  Chip,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import { useTranslation } from 'react-i18next';
import type { Collector, CollectorRun, EntityStats, SyncStats } from '../../api/types';
import ErrorBanner from '../../components/ErrorBanner';
import Modal from '../../components/Modal';
import { useCollectorRun, useCollectorRuns } from '../../hooks/useCollectors';
import { formatDateTime } from '../../lib/format';
import { runStatusColor, totals } from './collectorFormat';
import './CollectorRunsDialog.scss';

const STAT_KEYS: (keyof EntityStats)[] = [
  'created',
  'updated',
  'unchanged',
  'adopted',
  'deleted',
  'released',
  'skipped',
  'conflicts',
];
const ENTITIES: (keyof SyncStats)[] = ['streets', 'houseNumbers', 'geoObjects'];

function RunStatus({ run }: { run: CollectorRun }) {
  const { t } = useTranslation();
  const label = t(`collectors.runStatus.${run.status}`);
  return (
    <Chip
      size="small"
      color={runStatusColor[run.status]}
      label={run.phase ? `${label} · ${t(`collectors.phase.${run.phase}`)}` : label}
    />
  );
}

function StatsTable({ stats }: { stats: SyncStats }) {
  const { t } = useTranslation();
  return (
    <Table size="small" className="collector-runs-dialog__stats">
      <TableHead>
        <TableRow>
          <TableCell />
          {STAT_KEYS.map((k) => (
            <TableCell key={k} align="right">
              {t(`collectors.stats.${k}`)}
            </TableCell>
          ))}
        </TableRow>
      </TableHead>
      <TableBody>
        {ENTITIES.map((e) => (
          <TableRow key={e}>
            <TableCell>{t(`collectors.entities.${e}`)}</TableCell>
            {STAT_KEYS.map((k) => (
              <TableCell key={k} align="right">
                {stats[e][k]}
              </TableCell>
            ))}
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

/** The run's log. While new output arrives, it stays scrolled to the end,
 * unless the user scrolled up to read. */
function RunLog({ run }: { run: CollectorRun }) {
  const { t } = useTranslation();
  const ref = useRef<HTMLPreElement>(null);
  const atEnd = useRef(true);

  useLayoutEffect(() => {
    const el = ref.current;
    if (el && atEnd.current) el.scrollTop = el.scrollHeight;
  }, [run.log]);

  const running = run.status === 'running';
  return (
    <div>
      <Typography variant="subtitle2">{t('collectors.log')}</Typography>
      <pre
        ref={ref}
        className="collector-runs-dialog__log"
        onScroll={(e) => {
          const el = e.currentTarget;
          // A few pixels of slack, since scroll positions can be fractional.
          atEnd.current = el.scrollHeight - el.scrollTop - el.clientHeight < 8;
        }}
      >
        {run.log || t(running ? 'collectors.logWaiting' : 'collectors.logEmpty')}
      </pre>
    </div>
  );
}

function RunDetail({ runId, onCancel }: { runId: string; onCancel: (run: CollectorRun) => void }) {
  const { t, i18n } = useTranslation();
  const { data: run, error } = useCollectorRun(runId);
  if (!run) return <ErrorBanner message={error} />;

  const date = (iso: string) => formatDateTime(iso, i18n.language);
  return (
    <Stack spacing={2}>
      <ErrorBanner message={error} />
      <Stack direction="row" spacing={2} className="collector-runs-dialog__summary">
        <RunStatus run={run} />
        <Typography variant="body2" color="text.secondary">
          {t('collectors.startedBy', { date: date(run.startedAt), name: run.triggeredBy })}
          {run.finishedAt && ` · ${t('collectors.finished', { date: date(run.finishedAt) })}`}
          {run.exitCode !== undefined && ` · ${t('collectors.exitCode', { code: run.exitCode })}`}
        </Typography>
        {run.status === 'running' && (
          <Button size="small" color="warning" variant="outlined" onClick={() => onCancel(run)}>
            {t('collectors.cancel')}
          </Button>
        )}
      </Stack>
      {run.error && <ErrorBanner message={run.error} />}
      {run.stats && <StatsTable stats={run.stats} />}
      {run.issues && run.issues.length > 0 && (
        <div>
          <Typography variant="subtitle2">
            {t('collectors.issues', { count: run.issues.length })}
          </Typography>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>{t('collectors.issueKind')}</TableCell>
                <TableCell>{t('collectors.issueEntry')}</TableCell>
                <TableCell>{t('collectors.issueReason')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {run.issues.map((issue, i) => (
                <TableRow key={i}>
                  <TableCell>
                    <Chip
                      size="small"
                      variant="outlined"
                      label={t(`collectors.issueKinds.${issue.kind}`)}
                    />
                  </TableCell>
                  <TableCell>
                    {issue.key}
                    {issue.index !== undefined && (
                      <Typography variant="caption" color="text.secondary" component="div">
                        {t(`collectors.entities.${issue.entity}`)} #{issue.index}
                      </Typography>
                    )}
                  </TableCell>
                  <TableCell>{issue.reason}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
      <RunLog run={run} />
    </Stack>
  );
}

function RunList({
  collectorId,
  onOpen,
}: {
  collectorId: string;
  onOpen: (run: CollectorRun) => void;
}) {
  const { t, i18n } = useTranslation();
  const { data: runs, error, loading } = useCollectorRuns(collectorId);
  return (
    <>
      <ErrorBanner message={error} />
      <Table size="small">
        <TableHead>
          <TableRow>
            <TableCell>{t('collectors.started')}</TableCell>
            <TableCell>{t('collectors.status')}</TableCell>
            <TableCell>{t('collectors.triggeredBy')}</TableCell>
            <TableCell>{t('collectors.result')}</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {runs.map((run) => (
            <TableRow
              key={run.id}
              hover
              className="collector-runs-dialog__row"
              onClick={() => onOpen(run)}
            >
              <TableCell>{formatDateTime(run.startedAt, i18n.language)}</TableCell>
              <TableCell>
                <RunStatus run={run} />
              </TableCell>
              <TableCell>{run.triggeredBy}</TableCell>
              <TableCell>
                {run.stats ? t('collectors.summary', totals(run.stats)) : (run.error ?? '')}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {!loading && runs.length === 0 && (
        <Typography variant="body2" color="text.secondary" className="collector-runs-dialog__empty">
          {t('collectors.noRuns')}
        </Typography>
      )}
    </>
  );
}

/** A collector's run history; opening a run shows its counts, issues and log.
 * Running runs refresh until they end. */
export default function CollectorRunsDialog({
  collector,
  initialRunId,
  onClose,
  onCancel,
}: {
  collector: Collector | null;
  /** Opens straight at this run, e.g. one that was just started. */
  initialRunId?: string;
  onClose: () => void;
  onCancel: (run: CollectorRun) => void;
}) {
  const { t } = useTranslation();
  const [runId, setRunId] = useState<string | null>(initialRunId ?? null);
  if (!collector) return null;

  return (
    <Modal
      open
      title={t('collectors.historyTitle', { name: collector.name || collector.id })}
      onClose={onClose}
      headerExtra={
        runId && (
          <Button size="small" startIcon={<ArrowBackIcon />} onClick={() => setRunId(null)}>
            {t('collectors.allRuns')}
          </Button>
        )
      }
    >
      {runId ? (
        <RunDetail runId={runId} onCancel={onCancel} />
      ) : (
        <RunList collectorId={collector.id} onOpen={(run) => setRunId(run.id)} />
      )}
    </Modal>
  );
}
