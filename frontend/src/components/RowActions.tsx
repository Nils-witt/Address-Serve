import { IconButton, Tooltip } from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import { useTranslation } from 'react-i18next';
import CollectorChip from './CollectorChip';

/** A table row's edit and delete buttons; for a row a collector owns, which
 * can't be changed here, a chip naming the collector instead. */
export default function RowActions({
  collectorId,
  onEdit,
  onDelete,
}: {
  collectorId?: string;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  if (collectorId) return <CollectorChip collectorId={collectorId} />;
  return (
    <>
      <Tooltip title={t('common.edit')}>
        <IconButton size="small" onClick={onEdit}>
          <EditIcon fontSize="small" />
        </IconButton>
      </Tooltip>
      <Tooltip title={t('common.delete')}>
        <IconButton size="small" color="error" onClick={onDelete}>
          <DeleteIcon fontSize="small" />
        </IconButton>
      </Tooltip>
    </>
  );
}
