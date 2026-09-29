import { Button, Link, Paper, Stack, Typography } from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import { useTranslation } from 'react-i18next';
import type { Street } from '../../api/types';
import { formatCoordinates, osmUrl } from '../../lib/format';
import './StreetDetails.scss';

export default function StreetDetails({
  street,
  onEdit,
  onDelete,
}: {
  street: Street;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Paper className="street-details">
      <div className="street-details__text">
        <Typography variant="h5" component="h1">
          {street.name}
        </Typography>
        <Typography variant="body1" color="text.secondary">
          {[street.district, street.city, street.country].filter(Boolean).join(' · ')}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {t('fields.coordinates')}:{' '}
          <Link
            href={osmUrl(street.latitude, street.longitude)}
            target="_blank"
            rel="noopener noreferrer"
            className="street-details__coordinates"
          >
            {formatCoordinates(street.latitude, street.longitude)}
          </Link>
        </Typography>
      </div>
      <Stack direction="row" spacing={1} className="street-details__actions">
        <Button variant="outlined" startIcon={<EditIcon />} onClick={onEdit}>
          {t('common.edit')}
        </Button>
        <Button variant="outlined" color="error" startIcon={<DeleteIcon />} onClick={onDelete}>
          {t('common.delete')}
        </Button>
      </Stack>
    </Paper>
  );
}
