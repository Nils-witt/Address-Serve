import { Stack, TextField } from '@mui/material';
import { useTranslation } from 'react-i18next';

/** Latitude and longitude inputs side by side, kept as text so a half-typed
 * value ("52.", "-") isn't rejected while typing. */
export default function CoordinateFields({
  latitude,
  longitude,
  onLatitudeChange,
  onLongitudeChange,
}: {
  latitude: string;
  longitude: string;
  onLatitudeChange: (value: string) => void;
  onLongitudeChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <Stack direction="row" spacing={2}>
      <TextField
        label={t('fields.latitude')}
        size="small"
        required
        fullWidth
        inputMode="decimal"
        value={latitude}
        onChange={(e) => onLatitudeChange(e.target.value)}
      />
      <TextField
        label={t('fields.longitude')}
        size="small"
        required
        fullWidth
        inputMode="decimal"
        value={longitude}
        onChange={(e) => onLongitudeChange(e.target.value)}
      />
    </Stack>
  );
}
