import { Chip, Tooltip } from '@mui/material';
import SyncIcon from '@mui/icons-material/Sync';
import { useTranslation } from 'react-i18next';

/** Marks a row a collector owns, which only its runs may change. */
export default function CollectorChip({ collectorId }: { collectorId: string }) {
  const { t } = useTranslation();
  return (
    <Tooltip title={t('collectors.managedHint', { id: collectorId })}>
      <Chip
        icon={<SyncIcon />}
        label={t('collectors.managedBy', { id: collectorId })}
        size="small"
        variant="outlined"
      />
    </Tooltip>
  );
}
